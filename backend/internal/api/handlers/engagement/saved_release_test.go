package engagement

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func TestSaveReleaseHandler_RequiresAuthAndValidID(t *testing.T) {
	h := NewSavedReleaseHandler(nil)
	_, err := h.SaveReleaseHandler(context.Background(), &SaveReleaseRequest{ReleaseID: "1"})
	testhelpers.AssertHumaError(t, err, 401)

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err = h.SaveReleaseHandler(ctx, &SaveReleaseRequest{ReleaseID: "nope"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestSaveReleaseHandler_Success(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		SaveReleaseFn: func(userID, releaseID uint) error {
			if userID != 7 || releaseID != 42 {
				t.Fatalf("unexpected args: %d/%d", userID, releaseID)
			}
			return nil
		},
	}
	resp, err := NewSavedReleaseHandler(mock).SaveReleaseHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 7}),
		&SaveReleaseRequest{ReleaseID: "42"},
	)
	if err != nil || !resp.Body.Success {
		t.Fatalf("expected success, got resp=%+v err=%v", resp, err)
	}
}

func TestUnsaveReleaseHandler_RequiresAuthAndValidID(t *testing.T) {
	h := NewSavedReleaseHandler(nil)
	_, err := h.UnsaveReleaseHandler(context.Background(), &SaveReleaseRequest{ReleaseID: "1"})
	testhelpers.AssertHumaError(t, err, 401)

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	_, err = h.UnsaveReleaseHandler(ctx, &SaveReleaseRequest{ReleaseID: "nope"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUnsaveReleaseHandler_Success(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		UnsaveReleaseFn: func(userID, releaseID uint) error {
			if userID != 7 || releaseID != 42 {
				t.Fatalf("unexpected args: %d/%d", userID, releaseID)
			}
			return nil
		},
	}
	resp, err := NewSavedReleaseHandler(mock).UnsaveReleaseHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 7}),
		&SaveReleaseRequest{ReleaseID: "42"},
	)
	if err != nil || !resp.Body.Success {
		t.Fatalf("expected success, got resp=%+v err=%v", resp, err)
	}
}

func TestUnsaveReleaseHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		UnsaveReleaseFn: func(uint, uint) error { return errors.New("db error") },
	}
	_, err := NewSavedReleaseHandler(mock).UnsaveReleaseHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 7}),
		&SaveReleaseRequest{ReleaseID: "42"},
	)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestGetSavedReleasesHandler_ForwardsPage(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		GetUserSavedReleasesFn: func(userID uint, limit, offset int) ([]*contracts.SavedReleaseResponse, int64, error) {
			if userID != 3 || limit != 25 || offset != 5 {
				t.Fatalf("unexpected args: %d/%d/%d", userID, limit, offset)
			}
			return []*contracts.SavedReleaseResponse{{}}, 9, nil
		},
	}
	resp, err := NewSavedReleaseHandler(mock).GetSavedReleasesHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 3}),
		&GetSavedReleasesRequest{Limit: 25, Offset: 5},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 9 || len(resp.Body.Releases) != 1 {
		t.Fatalf("unexpected response: %+v", resp.Body)
	}
}

func TestGetReleaseSaveCountHandler_AnonymousAndAuthenticated(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		GetSaveCountFn:   func(releaseID uint) (int, error) { return 6, nil },
		IsReleaseSavedFn: func(userID, releaseID uint) (bool, error) { return true, nil },
	}
	h := NewSavedReleaseHandler(mock)
	anon, err := h.GetReleaseSaveCountHandler(context.Background(), &GetReleaseSaveCountRequest{ReleaseID: "8"})
	if err != nil || anon.Body.SaveCount != 6 || anon.Body.IsSaved {
		t.Fatalf("unexpected anonymous response: %+v err=%v", anon, err)
	}
	authed, err := h.GetReleaseSaveCountHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 2}),
		&GetReleaseSaveCountRequest{ReleaseID: "8"},
	)
	if err != nil || !authed.Body.IsSaved {
		t.Fatalf("unexpected authenticated response: %+v err=%v", authed, err)
	}
}

func TestGetReleaseSaveCountHandler_AuthenticatedStateError(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		GetSaveCountFn:   func(uint) (int, error) { return 6, nil },
		IsReleaseSavedFn: func(uint, uint) (bool, error) { return false, errors.New("db error") },
	}
	_, err := NewSavedReleaseHandler(mock).GetReleaseSaveCountHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 2}),
		&GetReleaseSaveCountRequest{ReleaseID: "8"},
	)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestBatchReleaseSaveCountsHandler_ZeroFillsAndAddsOwnState(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		GetBatchSaveCountsFn: func(ids []uint) (map[uint]int, error) {
			return map[uint]int{4: 2, 5: 0}, nil
		},
		GetSavedReleaseIDsFn: func(userID uint, ids []uint) (map[uint]bool, error) {
			return map[uint]bool{5: true}, nil
		},
	}
	req := &BatchReleaseSaveCountsRequest{}
	req.Body.ReleaseIDs = []int{4, 5}
	resp, err := NewSavedReleaseHandler(mock).BatchReleaseSaveCountsHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 2}), req,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Saves["4"].SaveCount != 2 || !resp.Body.Saves["5"].IsSaved {
		t.Fatalf("unexpected batch response: %+v", resp.Body.Saves)
	}
}

func TestBatchReleaseSaveCountsHandler_ValidatesBody(t *testing.T) {
	h := NewSavedReleaseHandler(&testhelpers.MockSavedReleaseService{})
	req := &BatchReleaseSaveCountsRequest{}
	req.Body.ReleaseIDs = []int{0}
	_, err := h.BatchReleaseSaveCountsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)

	req.Body.ReleaseIDs = make([]int, 201)
	_, err = h.BatchReleaseSaveCountsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestBatchReleaseSaveCountsHandler_AuthenticatedStateError(t *testing.T) {
	mock := &testhelpers.MockSavedReleaseService{
		GetBatchSaveCountsFn: func([]uint) (map[uint]int, error) {
			return map[uint]int{4: 2}, nil
		},
		GetSavedReleaseIDsFn: func(uint, []uint) (map[uint]bool, error) {
			return nil, errors.New("db error")
		},
	}
	req := &BatchReleaseSaveCountsRequest{}
	req.Body.ReleaseIDs = []int{4}
	_, err := NewSavedReleaseHandler(mock).BatchReleaseSaveCountsHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 2}), req,
	)
	testhelpers.AssertHumaError(t, err, 500)
}
