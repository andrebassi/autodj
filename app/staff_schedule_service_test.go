package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"autodj/domain/staffschedule"
	"autodj/ports/outbound"
)

var errStaffBoom = errors.New("staff boom")

// fakeStaffRepo is an in-memory StaffScheduleRepo with fault injection.
type fakeStaffRepo struct {
	shifts    map[string]staffschedule.Shift
	failList  bool
	failSave  bool
	failDel   bool
	deleteErr error
}

func newFakeStaffRepo() *fakeStaffRepo {
	return &fakeStaffRepo{shifts: map[string]staffschedule.Shift{}}
}

func (r *fakeStaffRepo) Save(_ context.Context, s staffschedule.Shift) error {
	if r.failSave {
		return errStaffBoom
	}
	r.shifts[s.ID] = s
	return nil
}

func (r *fakeStaffRepo) Get(_ context.Context, id string) (staffschedule.Shift, error) {
	s, ok := r.shifts[id]
	if !ok {
		return staffschedule.Shift{}, outbound.ErrShiftNotFound
	}
	return s, nil
}

func (r *fakeStaffRepo) Delete(_ context.Context, id string) error {
	if r.failDel {
		return errStaffBoom
	}
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if _, ok := r.shifts[id]; !ok {
		return outbound.ErrShiftNotFound
	}
	delete(r.shifts, id)
	return nil
}

func (r *fakeStaffRepo) List(_ context.Context) ([]staffschedule.Shift, error) {
	if r.failList {
		return nil, errStaffBoom
	}
	out := make([]staffschedule.Shift, 0, len(r.shifts))
	for _, s := range r.shifts {
		out = append(out, s)
	}
	return out, nil
}

var staffClock = fixedClock{t: time.Date(2026, 7, 20, 9, 30, 0, 0, time.UTC)} // Monday 09:30

func TestStaffAddShiftOK(t *testing.T) {
	repo := newFakeStaffRepo()
	svc := NewStaffScheduleService(repo, staffClock)

	sh, err := svc.AddShift(context.Background(), "a1", "Marina Vale", time.Monday, 8*60, 12*60)
	if err != nil {
		t.Fatalf("AddShift: %v", err)
	}
	if sh.ID == "" || sh.AnnouncerName != "Marina Vale" {
		t.Fatalf("unexpected shift: %+v", sh)
	}
	if len(repo.shifts) != 1 {
		t.Fatalf("expected 1 shift persisted, got %d", len(repo.shifts))
	}
	// two adds in the same nanosecond must differ by seq
	sh2, err := svc.AddShift(context.Background(), "a2", "Caio", time.Monday, 12*60, 16*60)
	if err != nil {
		t.Fatalf("AddShift 2: %v", err)
	}
	if sh2.ID == sh.ID {
		t.Fatalf("ids must be unique: %s", sh.ID)
	}
}

func TestStaffAddShiftValidation(t *testing.T) {
	svc := NewStaffScheduleService(newFakeStaffRepo(), staffClock)
	if _, err := svc.AddShift(context.Background(), "a1", "  ", time.Monday, 0, 60); !errors.Is(err, staffschedule.ErrEmptyAnnouncer) {
		t.Fatalf("expected ErrEmptyAnnouncer, got %v", err)
	}
	if _, err := svc.AddShift(context.Background(), "a1", "Marina", time.Monday, 600, 600); !errors.Is(err, staffschedule.ErrBadRange) {
		t.Fatalf("expected ErrBadRange, got %v", err)
	}
}

func TestStaffAddShiftOverlap(t *testing.T) {
	repo := newFakeStaffRepo()
	svc := NewStaffScheduleService(repo, staffClock)

	if _, err := svc.AddShift(context.Background(), "a1", "Marina", time.Monday, 8*60, 12*60); err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, err := svc.AddShift(context.Background(), "a2", "Caio", time.Monday, 10*60, 14*60)
	if !errors.Is(err, staffschedule.ErrOverlap) {
		t.Fatalf("expected ErrOverlap, got %v", err)
	}
	if len(repo.shifts) != 1 {
		t.Fatalf("overlapping shift must not persist, got %d", len(repo.shifts))
	}
}

func TestStaffAddShiftListError(t *testing.T) {
	repo := newFakeStaffRepo()
	repo.failList = true
	svc := NewStaffScheduleService(repo, staffClock)
	if _, err := svc.AddShift(context.Background(), "a1", "Marina", time.Monday, 8*60, 12*60); !errors.Is(err, errStaffBoom) {
		t.Fatalf("expected errStaffBoom, got %v", err)
	}
}

func TestStaffAddShiftSaveError(t *testing.T) {
	repo := newFakeStaffRepo()
	repo.failSave = true
	svc := NewStaffScheduleService(repo, staffClock)
	if _, err := svc.AddShift(context.Background(), "a1", "Marina", time.Monday, 8*60, 12*60); !errors.Is(err, errStaffBoom) {
		t.Fatalf("expected errStaffBoom, got %v", err)
	}
}

func TestStaffRemoveShift(t *testing.T) {
	repo := newFakeStaffRepo()
	svc := NewStaffScheduleService(repo, staffClock)
	sh, _ := svc.AddShift(context.Background(), "a1", "Marina", time.Monday, 8*60, 12*60)

	if err := svc.RemoveShift(context.Background(), sh.ID); err != nil {
		t.Fatalf("RemoveShift: %v", err)
	}
	if err := svc.RemoveShift(context.Background(), "nope"); !errors.Is(err, outbound.ErrShiftNotFound) {
		t.Fatalf("expected ErrShiftNotFound, got %v", err)
	}
}

func TestStaffRoster(t *testing.T) {
	repo := newFakeStaffRepo()
	svc := NewStaffScheduleService(repo, staffClock)
	_, _ = svc.AddShift(context.Background(), "a1", "Marina", time.Tuesday, 8*60, 12*60)
	_, _ = svc.AddShift(context.Background(), "a2", "Caio", time.Monday, 8*60, 12*60)

	roster, err := svc.Roster(context.Background())
	if err != nil {
		t.Fatalf("Roster: %v", err)
	}
	shifts := roster.Shifts()
	if len(shifts) != 2 || shifts[0].Weekday != time.Monday {
		t.Fatalf("expected Monday first, got %+v", shifts)
	}

	repo.failList = true
	if _, err := svc.Roster(context.Background()); !errors.Is(err, errStaffBoom) {
		t.Fatalf("expected errStaffBoom, got %v", err)
	}
}

func TestStaffOnAir(t *testing.T) {
	repo := newFakeStaffRepo()
	svc := NewStaffScheduleService(repo, staffClock)
	// Monday 08:00–12:00 covers the clock instant (Monday 09:30)
	_, _ = svc.AddShift(context.Background(), "a1", "Marina", time.Monday, 8*60, 12*60)

	sh, ok, err := svc.OnAir(context.Background(), staffClock.Now())
	if err != nil || !ok || sh.AnnouncerName != "Marina" {
		t.Fatalf("OnAir = %+v, %v, %v", sh, ok, err)
	}

	// a time nobody covers
	off := time.Date(2026, 7, 20, 5, 0, 0, 0, time.UTC) // Monday 05:00
	if _, ok, _ := svc.OnAir(context.Background(), off); ok {
		t.Fatalf("expected off air at 05:00")
	}

	repo.failList = true
	if _, _, err := svc.OnAir(context.Background(), staffClock.Now()); !errors.Is(err, errStaffBoom) {
		t.Fatalf("expected errStaffBoom, got %v", err)
	}
}
