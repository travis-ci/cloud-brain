package worker

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/provider"
)

func TestRefreshWorker(t *testing.T) {
	p := &provider.FakeProvider{}
	db := database.NewMemoryDatabase()

	inst, err := p.Create("standard-image")
	if err != nil {
		t.Fatal(err)
	}

	id, err := db.CreateInstance(database.Instance{
		Provider:   "fake",
		ProviderID: inst.ID,
		Image:      "standard-image",
		State:      "creating",
	})
	if err != nil {
		t.Fatal(err)
	}

	rw := &RefreshWorker{
		ProviderName: "fake",
		Provider:     p,
		DB:           db,
	}

	err = rw.RunOnce(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	dbInstance, err := db.GetInstance(id)
	if err != nil {
		t.Fatal(err)
	}

	if dbInstance.State != "starting" {
		t.Errorf("expected State = %q, but was %q", "starting", dbInstance.State)
	}

	p.MarkRunning(inst.ID)

	err = rw.RunOnce(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	dbInstance, err = db.GetInstance(id)
	if err != nil {
		t.Fatal(err)
	}

	if dbInstance.State != "running" {
		t.Errorf("expected State = %q, but was %q", "running", dbInstance.State)
	}
}
