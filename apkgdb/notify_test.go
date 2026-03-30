package apkgdb

import (
	"sync"
	"testing"
)

type mockNotifyTarget struct{}

func (m *mockNotifyTarget) NotifyInode(ino uint64, offt int64, data []byte) error {
	return nil
}

func TestNotifyTargetConcurrency(t *testing.T) {
	// This test verifies there is no data race when SetNotifyTarget and
	// notifyInode are called concurrently. Run with -race to detect races.
	d := &DB{}

	var wg sync.WaitGroup
	wg.Add(3)

	// Writer goroutine: repeatedly sets the notify target
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			d.SetNotifyTarget(&mockNotifyTarget{})
		}
	}()

	// Another writer goroutine: sets target to a different value
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			d.SetNotifyTarget(&mockNotifyTarget{})
		}
	}()

	// Reader goroutine: calls notifyInode concurrently
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = d.notifyInode(1, 0, nil)
		}
	}()

	wg.Wait()
}

func TestNotifyInodeWalksParent(t *testing.T) {
	parent := &DB{}
	parent.SetNotifyTarget(&mockNotifyTarget{})

	child := &DB{parent: parent}

	// child has no target set, should walk to parent
	err := child.notifyInode(1, 0, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNotifyInodeNoTarget(t *testing.T) {
	d := &DB{}

	// No target set, no parent — should return nil
	err := d.notifyInode(1, 0, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
