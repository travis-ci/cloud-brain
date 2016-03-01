package worker

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/provider"
)

func TestCreateWorker(t *testing.T) {
	p := &provider.FakeProvider{}
	db := database.NewMemoryDatabase()

	id, err := db.CreateInstance(database.Instance{
		Provider: "fake",
		Image:    "standard-image",
		State:    "creating",
	})
	if err != nil {
		t.Fatal(err)
	}

	cw := &CreateWorker{
		Provider: p,
		DB:       db,
	}

	err = cw.Work(context.TODO(), []byte(id))
	if err != nil {
		t.Fatal(err)
	}

	dbInstance, err := db.GetInstance(id)
	if err != nil {
		t.Fatal(err)
	}

	if dbInstance.State != "starting" {
		t.Errorf("expected State = %q, got %q", "starting", dbInstance.State)
	}
	if dbInstance.ProviderID == "" {
		t.Errorf("expected ProviderID to be non-empty, but was empty")
	}

	_, err = p.Get(dbInstance.ProviderID)
	if err != nil {
		t.Fatal(err)
	}
}
