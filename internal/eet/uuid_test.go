package eet_test

import (
	"testing"

	"github.com/google/uuid"
)

func newUUID(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}
