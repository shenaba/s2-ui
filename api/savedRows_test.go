package api

import (
	"path/filepath"
	"testing"

	"github.com/shenaba/2s-ui/database"
	"github.com/shenaba/2s-ui/database/model"
	"github.com/shenaba/2s-ui/logger"
	"github.com/shenaba/2s-ui/service"

	logging "github.com/op/go-logging"
)

func newSavedRowsDB(t *testing.T) {
	t.Helper()
	logger.InitLogger(logging.ERROR)
	if err := database.InitDB(filepath.Join(t.TempDir(), "saved.db")); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		if err := database.CloseDBForTest(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
}

// The reply describes the write. Rows ride along only for a single-row client
// write: a bulk action touches everything the caller submitted, and whole rows
// carry config and links, so echoing them back would resend the client table --
// with its credentials -- to a panel that reloads and never reads the reply.
func TestSavedRowsReturnsRowsOnlyForSingleClientWrites(t *testing.T) {
	newSavedRowsDB(t)

	seed := model.Client{
		Name: "probe", Enable: true,
		Inbounds: []byte(`[]`), Links: []byte(`[]`), Config: []byte(`{}`),
	}
	if err := database.GetDB().Create(&seed).Error; err != nil {
		t.Fatalf("seed client: %v", err)
	}

	a := ApiService{}
	cases := []struct {
		name     string
		result   service.SaveResult
		wantRows bool
	}{
		{"new returns the row", service.SaveResult{Object: "clients", Action: "new", Ids: []uint{seed.Id}}, true},
		{"edit returns the row", service.SaveResult{Object: "clients", Action: "edit", Ids: []uint{seed.Id}}, true},
		{"addbulk returns ids only", service.SaveResult{Object: "clients", Action: "addbulk", Ids: []uint{seed.Id}}, false},
		{"editbulk returns ids only", service.SaveResult{Object: "clients", Action: "editbulk", Ids: []uint{seed.Id}}, false},
		{"del returns ids only", service.SaveResult{Object: "clients", Action: "del", Ids: []uint{seed.Id}}, false},
		{"delbulk returns ids only", service.SaveResult{Object: "clients", Action: "delbulk", Ids: []uint{seed.Id}}, false},
		{"another object never returns rows", service.SaveResult{Object: "inbounds", Action: "new"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := a.savedRows(&tc.result)
			if err != nil {
				t.Fatalf("savedRows: %v", err)
			}
			if resp["object"] != tc.result.Object || resp["action"] != tc.result.Action {
				t.Errorf("object/action not echoed: %v", resp)
			}
			rows, has := resp["clients"]
			if has != tc.wantRows {
				t.Fatalf("clients present = %v, want %v", has, tc.wantRows)
			}
			if tc.wantRows && len(rows.([]model.Client)) != 1 {
				t.Errorf("expected one row, got %v", rows)
			}
		})
	}
}

// A row can be deleted between the save's commit and this read. The scan then
// leaves a nil slice, which marshals as null -- so a caller indexing into
// clients[0] would crash on the one response it never expected.
func TestSavedRowsAnswersEmptyArrayWhenTheRowIsGone(t *testing.T) {
	newSavedRowsDB(t)

	a := ApiService{}
	resp, err := a.savedRows(&service.SaveResult{Object: "clients", Action: "new", Ids: []uint{4242}})
	if err != nil {
		t.Fatalf("savedRows: %v", err)
	}
	rows, has := resp["clients"]
	if !has {
		t.Fatal("clients key missing entirely")
	}
	list, ok := rows.([]model.Client)
	if !ok {
		t.Fatalf("clients is %T, want []model.Client (a nil pointer would marshal as null)", rows)
	}
	if list == nil {
		t.Error("clients is a nil slice; it marshals as null rather than []")
	}
	if len(list) != 0 {
		t.Errorf("expected no rows, got %d", len(list))
	}
}
