package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/qwerin/nanofaktura/internal/auth"
	"github.com/qwerin/nanofaktura/internal/config"
	"github.com/qwerin/nanofaktura/internal/db"
	"github.com/qwerin/nanofaktura/internal/model"
)

const userUsage = `usage:
  nanofaktura user reset-2fa --email <email>

reset-2fa turns off two-factor authentication of a user who lost both the
second factor and the recovery codes (they log in with the password only and
can set it up again). The database comes from the NANOFAKTURA_* environment.`

// runUser runs "nanofaktura user …" (args after "user").
func runUser(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(userUsage)
	}
	switch args[0] {
	case "reset-2fa":
		return userReset2FA(ctx, args[1:], stdout)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, userUsage)
		return nil
	}
	return fmt.Errorf("unknown user command %q\n%s", args[0], userUsage)
}

func userReset2FA(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("user reset-2fa", flag.ContinueOnError)
	email := fs.String("email", "", "e-mail of the user")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" {
		return errors.New("--email is required\n" + userUsage)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	gdb, err := db.Open(cfg.DBDriver, cfg.DBDSN, db.WithLogging(cfg.DBLog, time.Duration(cfg.DBSlowMS)*time.Millisecond))
	if err != nil {
		return err
	}
	if err := db.Migrate(gdb); err != nil {
		return err
	}
	var user model.User
	if err := gdb.WithContext(ctx).Where("email = ?", strings.ToLower(strings.TrimSpace(*email))).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no user with e-mail %s", *email)
		}
		return err
	}
	if err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return auth.ResetSecondFactor(tx, user.ID) }); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "two-factor authentication of %s is off\n", user.Email)
	return nil
}
