//go:build diff_fast

package external

import models "github.com/2comjie/nova/internal/diffgen/testdata/basic"

type Model struct {
	Child    *models.Child            `diff:"1"`
	Children map[uint64]*models.Child `diff:"2"`
	Order    []*models.Child          `diff:"3"`
	Basic    *models.Model            `diff:"4"`
}
