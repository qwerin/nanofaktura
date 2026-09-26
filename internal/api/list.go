package api

import "gorm.io/gorm"

// PageParams are the ?page=&per_page= query parameters; embed in list inputs.
type PageParams struct {
	Page    int `query:"page" minimum:"1" default:"1"`
	PerPage int `query:"per_page" minimum:"1" maximum:"200" default:"50"`
}

// ListResponse is the envelope of every list endpoint.
type ListResponse[T any] struct {
	Items   []T   `json:"items" nullable:"false"`
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}

// paginate counts and loads one page of M rows from q (filters and ordering
// already applied) and converts them with conv. If q has no Model/Table, M's
// table is used.
func paginate[M, T any](q *gorm.DB, p PageParams, conv func(*M) T) (*Out[ListResponse[T]], error) {
	if q.Statement.Model == nil && q.Statement.Table == "" {
		q = q.Model(new(M))
	}
	q = q.Session(&gorm.Session{})

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, dbErr(err, "records")
	}
	var rows []M
	if err := q.Offset((p.Page - 1) * p.PerPage).Limit(p.PerPage).Find(&rows).Error; err != nil {
		return nil, dbErr(err, "records")
	}
	items := make([]T, len(rows))
	for i := range rows {
		items[i] = conv(&rows[i])
	}
	return &Out[ListResponse[T]]{Body: ListResponse[T]{Items: items, Page: p.Page, PerPage: p.PerPage, Total: total}}, nil
}
