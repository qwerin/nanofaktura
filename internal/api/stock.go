package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/billing"
	"github.com/qwerin/nanofaktura/internal/model"
)

// Stock (SPEC §7.1): every change of PriceItem.stock_quantity goes through a
// StockMove written in the same transaction (addStockMove / deleteStockMoves),
// so stock_quantity = Σ moves at all times.
//
// Documents write their moves themselves: syncInvoiceStock / syncExpenseStock
// delete the document's moves and rewrite them from its current lines, which
// covers create, PATCH, cancel/undo_cancel and delete alike.

// StockMove is one receipt (in) or issue (out) of a price item.
type StockMove struct {
	ID          uint      `json:"id"`
	PriceItemID uint      `json:"price_item_id"`
	Direction   string    `json:"direction" enum:"in,out"`
	Quantity    string    `json:"quantity" example:"2" doc:"Always positive; the sign is direction"`
	MovedOn     string    `json:"moved_on"`
	Note        string    `json:"note"`
	InvoiceID   *uint     `json:"invoice_id,omitempty" doc:"Generated from this invoice (not deletable manually)"`
	ExpenseID   *uint     `json:"expense_id,omitempty" doc:"Generated from this expense (not deletable manually)"`
	CreatedAt   time.Time `json:"created_at"`
}

func toStockMove(m *model.StockMove) StockMove {
	return StockMove{
		ID: m.ID, PriceItemID: m.PriceItemID, Direction: m.Direction, Quantity: billing.FormatQuantity(m.QuantityMilli),
		MovedOn: m.MovedOn, Note: m.Note, InvoiceID: m.InvoiceID, ExpenseID: m.ExpenseID, CreatedAt: m.CreatedAt,
	}
}

type StockMoveCreate struct {
	Direction string `json:"direction" enum:"in,out"`
	Quantity  string `json:"quantity" minLength:"1" maxLength:"20" example:"5" doc:"Positive decimal, max 3 places"`
	MovedOn   string `json:"moved_on,omitempty" format:"date" doc:"Default today"`
	Note      string `json:"note,omitempty" maxLength:"500"`
}

// StockMoveResult is the created move and the updated price item.
type StockMoveResult struct {
	Move      StockMove `json:"move"`
	PriceItem PriceItem `json:"price_item"`
}

func (s *server) registerStockMoves(g huma.API) {
	huma.Get(g, "/price-items/{id}/stock-moves", s.listStockMoves)
	huma.Post(g, "/price-items/{id}/stock-moves", s.createStockMove, status(http.StatusCreated), auth.ForEditors)
	huma.Delete(g, "/price-items/{id}/stock-moves/{move_id}", s.deleteStockMove, status(http.StatusNoContent), auth.ForEditors)
}

func (s *server) listStockMoves(ctx context.Context, in *struct {
	ID uint `path:"id"`
	PageParams
}) (*Out[ListResponse[StockMove]], error) {
	if _, err := loadPriceItem(ctx, s.db.WithContext(ctx), in.ID); err != nil {
		return nil, err
	}
	q := s.scoped(ctx).Model(&model.StockMove{}).Where("price_item_id = ?", in.ID).Order("moved_on DESC, id DESC")
	return paginate(q, in.PageParams, toStockMove)
}

func (s *server) createStockMove(ctx context.Context, in *struct {
	ID   uint `path:"id"`
	Body StockMoveCreate
}) (*Out[StockMoveResult], error) {
	var res StockMoveResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item, err := loadPriceItem(ctx, tx, in.ID)
		if err != nil {
			return err
		}
		if !item.TrackStock {
			return conflict("stock is not tracked for this price item; enable track_stock first")
		}
		q, err := billing.ParseQuantity(in.Body.Quantity)
		if err != nil {
			return invalid("quantity", err.Error())
		}
		if q <= 0 {
			return invalid("quantity", "quantity must be positive")
		}
		mv := &model.StockMove{
			AccountID: auth.AccountFrom(ctx).ID, PriceItemID: item.ID, Direction: in.Body.Direction,
			QuantityMilli: q, MovedOn: defaultStr(in.Body.MovedOn, s.today()), Note: in.Body.Note,
		}
		if !billing.ValidDate(mv.MovedOn) {
			return invalid("moved_on", "invalid date")
		}
		if err := addStockMove(tx, mv); err != nil {
			return err
		}
		if item, err = loadPriceItem(ctx, tx, item.ID); err != nil {
			return err
		}
		if err := recordStockMove(ctx, tx, item, mv, false); err != nil {
			return err
		}
		res = StockMoveResult{Move: toStockMove(mv), PriceItem: toPriceItem(item)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Out[StockMoveResult]{Body: res}, nil
}

func (s *server) deleteStockMove(ctx context.Context, in *struct {
	ID     uint `path:"id"`
	MoveID uint `path:"move_id"`
}) (*NoContent, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var mv model.StockMove
		if err := tx.Scopes(inAccount(ctx)).Where("price_item_id = ?", in.ID).First(&mv, in.MoveID).Error; err != nil {
			return dbErr(err, "stock move")
		}
		if mv.InvoiceID != nil || mv.ExpenseID != nil {
			return conflict("this stock move was generated from a document; change the document instead")
		}
		if err := deleteStockMoves(tx, []model.StockMove{mv}); err != nil {
			return err
		}
		item, err := loadPriceItem(ctx, tx, mv.PriceItemID)
		if err != nil {
			return err
		}
		return recordStockMove(ctx, tx, item, &mv, true)
	})
	if err != nil {
		return nil, err
	}
	return &NoContent{}, nil
}

// ---- stock bookkeeping ----

// addStockMove stores mv and applies it to the item's stock quantity.
func addStockMove(tx *gorm.DB, mv *model.StockMove) error {
	if err := tx.Create(mv).Error; err != nil {
		return dbErr(err, "stock move")
	}
	return adjustStock(tx, mv.PriceItemID, mv.Delta())
}

// deleteStockMoves deletes moves and reverts their effect on stock quantities.
func deleteStockMoves(tx *gorm.DB, moves []model.StockMove) error {
	for i := range moves {
		if err := tx.Delete(&model.StockMove{}, moves[i].ID).Error; err != nil {
			return dbErr(err, "stock move")
		}
		if err := adjustStock(tx, moves[i].PriceItemID, -moves[i].Delta()); err != nil {
			return err
		}
	}
	return nil
}

func adjustStock(tx *gorm.DB, itemID uint, delta int64) error {
	err := tx.Model(&model.PriceItem{}).Where("id = ?", itemID).
		UpdateColumn("stock_quantity_milli", gorm.Expr("stock_quantity_milli + ?", delta)).Error
	return dbErrOrNil(err, "price item")
}

// stockLine is the part of a document line relevant for stock.
type stockLine struct {
	PriceItemID   *uint
	QuantityMilli int64
}

// docStock identifies the document whose moves are rewritten: column is
// "invoice_id" or "expense_id".
type docStock struct {
	column string
	id     uint
}

// rewriteDocStock deletes the document's moves and, when active, writes new
// ones for its lines with a stock-tracked price item: a positive quantity
// moves in direction dir, a negative one in the opposite direction (so a
// correction with negative quantities returns goods to stock).
func rewriteDocStock(ctx context.Context, tx *gorm.DB, doc docStock, active bool, movedOn, dir string, lines []stockLine) error {
	var old []model.StockMove
	if err := tx.Scopes(inAccount(ctx)).Where(doc.column+" = ?", doc.id).Find(&old).Error; err != nil {
		return dbErr(err, "stock move")
	}
	if err := deleteStockMoves(tx, old); err != nil {
		return err
	}
	touched := make([]uint, 0, len(old)+len(lines))
	for _, mv := range old {
		touched = append(touched, mv.PriceItemID)
	}
	if !active {
		return syncStockTodos(ctx, tx, touched)
	}
	ids := []uint{}
	for _, l := range lines {
		if l.PriceItemID != nil {
			ids = append(ids, *l.PriceItemID)
		}
	}
	if len(ids) == 0 {
		return syncStockTodos(ctx, tx, touched)
	}
	var tracked []model.PriceItem
	if err := tx.Scopes(inAccount(ctx)).Select("id").Where("id IN ? AND track_stock = ?", ids, true).Find(&tracked).Error; err != nil {
		return dbErr(err, "price item")
	}
	isTracked := map[uint]bool{}
	for _, t := range tracked {
		isTracked[t.ID] = true
	}
	opposite := map[string]string{model.StockIn: model.StockOut, model.StockOut: model.StockIn}
	for _, l := range lines {
		if l.PriceItemID == nil || !isTracked[*l.PriceItemID] || l.QuantityMilli == 0 {
			continue
		}
		mv := &model.StockMove{AccountID: auth.AccountFrom(ctx).ID, PriceItemID: *l.PriceItemID,
			Direction: dir, QuantityMilli: l.QuantityMilli, MovedOn: movedOn}
		if l.QuantityMilli < 0 {
			mv.Direction, mv.QuantityMilli = opposite[dir], -l.QuantityMilli
		}
		id := doc.id
		if doc.column == "invoice_id" {
			mv.InvoiceID = &id
		} else {
			mv.ExpenseID = &id
		}
		if err := addStockMove(tx, mv); err != nil {
			return err
		}
		touched = append(touched, mv.PriceItemID)
	}
	return syncStockTodos(ctx, tx, touched) // low stock todo + stock.low
}

// syncInvoiceStock rewrites the stock moves of an invoice (loaded with lines):
// issued invoices and corrections move stock out (corrections with negative
// quantities return it), proformas never, cancelled documents not at all.
func syncInvoiceStock(ctx context.Context, tx *gorm.DB, m *model.Invoice) error {
	active := (m.DocumentType == model.DocInvoice || m.DocumentType == model.DocCorrection) &&
		m.Status != model.StatusCancelled
	lines := make([]stockLine, len(m.Lines))
	for i, l := range m.Lines {
		lines[i] = stockLine{PriceItemID: l.PriceItemID, QuantityMilli: l.QuantityMilli}
	}
	return rewriteDocStock(ctx, tx, docStock{"invoice_id", m.ID}, active, m.IssuedOn, model.StockOut, lines)
}

// clearInvoiceStock reverts all stock moves of a deleted invoice.
func clearInvoiceStock(ctx context.Context, tx *gorm.DB, id uint) error {
	return rewriteDocStock(ctx, tx, docStock{"invoice_id", id}, false, "", "", nil)
}

// checkPriceItems validates that every price_item_id of lines belongs to the
// current account (422 on lines[i].price_item_id otherwise).
func checkPriceItems(ctx context.Context, tx *gorm.DB, lines []InvoiceLineInput) error {
	ids := []uint{}
	for _, l := range lines {
		if l.PriceItemID != nil {
			ids = append(ids, *l.PriceItemID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var found []uint
	if err := tx.Model(&model.PriceItem{}).Scopes(inAccount(ctx)).Where("id IN ?", ids).Pluck("id", &found).Error; err != nil {
		return dbErr(err, "price item")
	}
	ok := map[uint]bool{}
	for _, id := range found {
		ok[id] = true
	}
	for i, l := range lines {
		if l.PriceItemID != nil && !ok[*l.PriceItemID] {
			return invalid(fmt.Sprintf("lines[%d].price_item_id", i), "price item not found")
		}
	}
	return nil
}
