package store

import (
	"testing"
	"time"
)

func TestListNewestFirstAndReload(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	for _, offset := range []time.Duration{time.Second, 3 * time.Second, 2 * time.Second} {
		if _, err := s.Save(Job{ReceivedAt: base.Add(offset), RemoteAddr: "1.2.3.4:5", Data: []byte("^XA^XZ"), Images: [][]byte{{1}}}); err != nil {
			t.Fatal(err)
		}
	}
	// Same timestamp twice must not overwrite.
	if _, err := s.Save(Job{ReceivedAt: base, RemoteAddr: "1.2.3.4:5", Data: []byte("^XA^XZ")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(Job{ReceivedAt: base, RemoteAddr: "1.2.3.4:5", Data: []byte("^XA^XZ")}); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	list := reopened.List()
	if len(list) != 5 {
		t.Fatalf("got %d prints, want 5", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i].ReceivedAt.After(list[i-1].ReceivedAt) {
			t.Fatalf("not sorted desc: %v before %v", list[i-1].ReceivedAt, list[i].ReceivedAt)
		}
	}
}

func TestRejectsUnknownIDs(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImagePath("../../etc", "passwd"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	if err := s.Delete(".."); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
