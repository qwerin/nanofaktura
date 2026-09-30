package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/backup"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
	"github.com/qwerin/nanofaktura/internal/storage"
)

const backupUsage = `usage:
  nanofaktura backup export --account <slug> [--out file.zip]   (--out - = stdout)
  nanofaktura backup import --owner <email> [--name "Název"] file.zip

The database, data directory and limits come from the NANOFAKTURA_* environment
(as for the server). Import always creates a new account owned by --owner.`

// runBackup runs "nanofaktura backup …" (args after "backup").
func runBackup(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(backupUsage)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	switch args[0] {
	case "export":
		return backupExport(ctx, cfg, args[1:], stdout, stderr)
	case "import":
		return backupImport(ctx, cfg, args[1:], stdout)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, backupUsage)
		return nil
	}
	return fmt.Errorf("unknown backup command %q\n%s", args[0], backupUsage)
}

func openDB(cfg config.Config) (*gorm.DB, storage.Storage, error) {
	gdb, err := db.Open(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		return nil, nil, err
	}
	if err := db.Migrate(gdb); err != nil {
		return nil, nil, err
	}
	return gdb, storage.NewLocal(filepath.Join(cfg.DataDir, "attachments")), nil
}

// parseFlags parses flags given before and after the positional arguments.
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func backupExport(ctx context.Context, cfg config.Config, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("backup export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	slug := fs.String("account", "", "slug of the account to export")
	out := fs.String("out", "", "output file (default nanofaktura-<slug>-<date>.zip, - = stdout)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *slug == "" {
		return errors.New("--account is required\n" + backupUsage)
	}
	gdb, st, err := openDB(cfg)
	if err != nil {
		return err
	}
	var acc model.Account
	if err := gdb.Where("slug = ?", *slug).First(&acc).Error; err != nil {
		return fmt.Errorf("account %q: %w", *slug, err)
	}
	now := time.Now()
	path := *out
	if path == "" {
		path = backup.Filename(acc.Slug, now)
	}
	var w io.Writer = stdout
	var f *os.File
	if path != "-" {
		if f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err != nil {
			return err
		}
		w = f
	}
	man, err := backup.Export(ctx, gdb, st, acc.ID, w)
	if f != nil {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}
	if err != nil {
		return err
	}
	if err := backup.RecordExported(gdb, &acc, nil, now, man); err != nil {
		return err
	}
	if path != "-" {
		fmt.Fprintf(stdout, "exported account %s to %s (%s)\n", acc.Slug, path, formatCounts(man.Counts))
	}
	return nil
}

func backupImport(ctx context.Context, cfg config.Config, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup import", flag.ContinueOnError)
	owner := fs.String("owner", "", "e-mail of the existing user who becomes the owner")
	name := fs.String("name", "", "name of the new account (default: name from the backup)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if *owner == "" || len(pos) != 1 {
		return errors.New("--owner and one backup file are required\n" + backupUsage)
	}
	gdb, st, err := openDB(cfg)
	if err != nil {
		return err
	}
	var user model.User
	if err := gdb.Where("email = ?", strings.ToLower(strings.TrimSpace(*owner))).First(&user).Error; err != nil {
		return fmt.Errorf("user %q: %w (register the user first)", *owner, err)
	}
	f, err := os.Open(pos[0])
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	acc, warnings, err := backup.Import(ctx, gdb, st, f, info.Size(), backup.ImportOptions{
		OwnerUserID: user.ID, Name: *name, Now: time.Now(), MaxBytes: int64(cfg.ImportMaxMB) << 20,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "imported as account %s (%s), owner %s\n", acc.Slug, acc.Name, user.Email)
	for _, w := range warnings {
		if w.Count > 0 {
			fmt.Fprintf(stdout, "warning: %s (%d)\n", w.Message, w.Count)
		} else {
			fmt.Fprintf(stdout, "warning: %s\n", w.Message)
		}
	}
	return nil
}

func formatCounts(c map[string]int) string {
	parts := []string{}
	for _, k := range []string{"invoices", "expenses", "subjects", "attachments"} {
		parts = append(parts, fmt.Sprintf("%s %d", k, c[k]))
	}
	return strings.Join(parts, ", ")
}
