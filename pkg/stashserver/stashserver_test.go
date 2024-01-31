package stashserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const TestFilePrefix = "testPrefix"

type testStasher struct {
	stashData string
}

func (d *testStasher) Stash(w io.Writer) error {
	w.Write([]byte(d.stashData))
	return nil
}

type testErrorStasher struct {
}

func (d *testErrorStasher) Stash(w io.Writer) error {
	return fmt.Errorf("Very bad dangerous error")
}

type testWaitStasher struct {
}

func (d *testWaitStasher) Stash(w io.Writer) error {
	time.Sleep(3 * time.Second)
	return nil
}

func TestGetStashes(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	validateGetStashes(t, ds, 0)
}

func TestCreateStash(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	mustCreateStash(t, ds)
	validateStashCreated(t, 1, testdir)
}

func TestCreateAndGetStash(t *testing.T) {
	ds, testStasher, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	expectednumStashes := 1
	mustCreateStash(t, ds)
	validateStashCreated(t, expectednumStashes, testdir)

	stashes := validateGetStashes(t, ds, expectednumStashes)
	var stashName string
	for stash := range stashes {
		stashName = stash
	}
	validateGetStash(t, ds, stashName, []byte(testStasher.stashData))
}

func TestGetStashStashInvalidMethod(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	rr := httptest.NewRecorder()

	request, err := http.NewRequest("POST", "/stashes/stash", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestGetStashNonExistentStash(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/stashes/fakestashname", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusNotFound)
	}
}

func TestGetErroredStash(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)
	ds.stasher = &testErrorStasher{}

	mustCreateStash(t, ds)
	time.Sleep(100 * time.Millisecond)
	stashes := validateGetStashes(t, ds, 1)

	var stash *Stash
	for _, d := range stashes {
		stash = d
	}

	if stash.Status != StashFailed {
		t.Errorf("Expected stash to have status: %s", StashFailed)
	}
	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/stashes/"+stash.Name, nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusNotFound)
	}
}

func TestGetBuffer(t *testing.T) {
	ds, testStasher, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/buffer", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	responseBody, err := io.ReadAll(rr.Result().Body)
	if !bytes.Equal(responseBody, []byte(testStasher.stashData)) {
		t.Errorf("data expected in stash: %s , data found: %s", testStasher.stashData, string(responseBody))
	}
}

func TestInvalidGetBuffer(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	rr := httptest.NewRecorder()

	request, err := http.NewRequest("POST", "/buffer", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestGetBufferStashError(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	ds.stasher = &testErrorStasher{}
	defer os.RemoveAll(testdir)

	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/buffer", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusInternalServerError)
	}
}

func TestCreateBufferStash(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	ds.CreateBufferStash()

	validateStashCreated(t, 1, testdir)
}

func TestStashCompletionFunc(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	count := 0
	ds.AddCompletionCallback(func(d *Stash) { count++ })

	mustCreateStash(t, ds)
	time.Sleep(time.Second)
	mustCreateStash(t, ds)

	// Wait for file IO

	if count != 2 {
		t.Errorf("Expected the callback to be called twice")
	}
}

func TestLoadingExistingStashes(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	if numStashes := len(ds.stashes); numStashes != 0 {
		t.Errorf("expected ds to start with 0 stashes but found: %v", numStashes)
	}

	stashesToLoad := 3
	stashNames := map[string]bool{}
	for i := 0; i < stashesToLoad; i++ {
		time.Sleep(time.Second)
		stashName := fmt.Sprintf("%s-%v", TestFilePrefix, i)
		stashNames[stashName] = true
		stashFile := stashName + ".json"
		f, err := os.Create(filepath.Join(testdir, stashFile))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	ds.loadExistingFileStashes()
	if numStashes := len(ds.stashes); numStashes != 3 {
		t.Errorf("expected ds to have %v stahes but found: %v", stashesToLoad, numStashes)
	}

	for stash := range stashNames {
		if _, ok := ds.stashes[stash]; !ok {
			t.Errorf("expected ds to have stash %s", stash)
		}
	}
}

func TestMaxStashes(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	if numStashes := len(ds.stashes); numStashes != 0 {
		t.Errorf("expected ds to start with 0 stashes but found: %v", numStashes)
	}
	ds.maxStashes = 2

	stashesToTake := 5

	stashNames := make([]string, stashesToTake)
	for i := 0; i < stashesToTake; i++ {
		time.Sleep(time.Second)
		rr := mustCreateStash(t, ds)
		validateStashCreated(t, int(math.Min(float64(ds.maxStashes), float64(i+1))), testdir)
		stashName, err := io.ReadAll(rr.Result().Body)
		if err != nil {
			t.Fatal(err)
			stashNames[i] = string(stashName)
		}
	}

}

func initTestEnv(t *testing.T) (*StashServer, *testStasher, string) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testStasher := &testStasher{"data"}
	ds := NewStashServer(testStasher, 10)
	dir, err := os.MkdirTemp("", "testtmp")
	if err != nil {
		t.Fatal(err)
	}

	ds.stashDir = dir
	ds.stashPrefix = TestFilePrefix
	return ds, testStasher, dir
}

func mustCreateStash(t *testing.T, ds *StashServer) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/stashes", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)
	return rr
}

func validateStashCreated(t *testing.T, expectedNumStashes int, dir string) {
	// Wait a bit for file IO to happen
	time.Sleep(600 * time.Millisecond)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	filesFound := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), TestFilePrefix) {
			filesFound++
		}
	}

	if filesFound != expectedNumStashes {
		t.Errorf("stash server created expected to create %v stash but got %v", expectedNumStashes, filesFound)
	}
}

func validateGetStashes(t *testing.T, ds *StashServer, expectedNumStashes int) map[string]*Stash {
	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/stashes", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusCreated)
	}

	stashes := make(map[string]*Stash)

	responseBody, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(responseBody, &stashes)

	if stashesLen := len(stashes); stashesLen != expectedNumStashes {
		t.Errorf("unexpected stashes found, expected:%v  got: %v", expectedNumStashes, stashesLen)
	}
	return stashes
}

func validateGetStash(t *testing.T, ds *StashServer, stashName string, expectedData []byte) {
	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/stashes/"+stashName, nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusCreated)
	}

	responseBody, err := io.ReadAll(rr.Result().Body)
	if !bytes.Equal(responseBody, expectedData) {
		t.Errorf("data expected in stash: %s , data found: %s", string(expectedData), string(responseBody))
	}
}
