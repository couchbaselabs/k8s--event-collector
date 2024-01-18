package dumpserver

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

type testDumper struct {
	dumpData string
}

func (d *testDumper) Dump(w io.Writer) error {
	w.Write([]byte(d.dumpData))
	return nil
}

type testErrorDumper struct {
}

func (d *testErrorDumper) Dump(w io.Writer) error {
	return fmt.Errorf("Very bad dangerous error")
}

type testWaitDumper struct {
}

func (d *testWaitDumper) Dump(w io.Writer) error {
	time.Sleep(3 * time.Second)
	return nil
}

func TestGetDumps(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	validateGetDumps(t, ds, 0)
}

func TestCreateDump(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	mustCreateDump(t, ds)
	validateDumpCreated(t, 1, testdir)
}

func TestCreateAndGetDump(t *testing.T) {
	ds, testDumper, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	expectedNumDumps := 1
	mustCreateDump(t, ds)
	validateDumpCreated(t, expectedNumDumps, testdir)

	dumps := validateGetDumps(t, ds, expectedNumDumps)
	var dumpName string
	for dump := range dumps {
		dumpName = dump
	}
	validateGetDump(t, ds, dumpName, []byte(testDumper.dumpData))
}

func TestGetDumpInvalidMethod(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	rr := httptest.NewRecorder()

	request, err := http.NewRequest("POST", "/dumps/dump", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusCreated)
	}
}

func TestGetDumpNonExistentDump(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/dumps/fakedumpname", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusNotFound)
	}
}

func TestGetErroredDump(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)
	ds.dumper = &testErrorDumper{}

	mustCreateDump(t, ds)
	time.Sleep(100 * time.Millisecond)
	dumps := validateGetDumps(t, ds, 1)

	var dump *Dump
	for _, d := range dumps {
		dump = d
	}

	if dump.Status != DumpFailed {
		t.Errorf("Expected dump to have status: %s", DumpFailed)
	}
	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/dumps/"+dump.Name, nil)
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
	ds, testDumper, testdir := initTestEnv(t)
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
	if !bytes.Equal(responseBody, []byte(testDumper.dumpData)) {
		t.Errorf("data expected in dump: %s , data found: %s", testDumper.dumpData, string(responseBody))
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

func TestGetBufferDumpError(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	ds.dumper = &testErrorDumper{}
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

func TestCreateBufferDump(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	ds.CreateBufferDump()

	validateDumpCreated(t, 1, testdir)
}

func TestDumpCompletionFunc(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	count := 0
	ds.AddCompletionCallback(func(d *Dump) { count++ })

	mustCreateDump(t, ds)
	time.Sleep(time.Second)
	mustCreateDump(t, ds)

	// Wait for file IO

	if count != 2 {
		t.Errorf("Expected the callback to be called twice")
	}
}

func TestLoadingExistingDumps(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	if numDumps := len(ds.dumps); numDumps != 0 {
		t.Errorf("expected ds to start with 0 dumps but found: %v", numDumps)
	}

	dumpsToLoad := 3
	dumpNames := map[string]bool{}
	for i := 0; i < dumpsToLoad; i++ {
		time.Sleep(time.Second)
		dumpName := fmt.Sprintf("%s-%v", TestFilePrefix, i)
		dumpNames[dumpName] = true
		dumpFile := dumpName + ".json"
		f, err := os.Create(filepath.Join(testdir, dumpFile))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	ds.loadExistingFileDumps()
	if numDumps := len(ds.dumps); numDumps != 3 {
		t.Errorf("expected ds to have %v dumps but found: %v", dumpsToLoad, numDumps)
	}

	for dump := range dumpNames {
		if _, ok := ds.dumps[dump]; !ok {
			t.Errorf("expected ds to have dump %s", dump)
		}
	}
}

func TestMaxDumps(t *testing.T) {
	ds, _, testdir := initTestEnv(t)
	defer os.RemoveAll(testdir)

	if numDumps := len(ds.dumps); numDumps != 0 {
		t.Errorf("expected ds to start with 0 dumps but found: %v", numDumps)
	}
	ds.maxDumps = 2

	dumpsToTake := 5

	dumpNames := make([]string, dumpsToTake)
	for i := 0; i < dumpsToTake; i++ {
		time.Sleep(time.Second)
		rr := mustCreateDump(t, ds)
		validateDumpCreated(t, int(math.Min(float64(ds.maxDumps), float64(i+1))), testdir)
		dumpName, err := io.ReadAll(rr.Result().Body)
		if err != nil {
			t.Fatal(err)
			dumpNames[i] = string(dumpName)
		}
	}

}

func initTestEnv(t *testing.T) (*DumpServer, *testDumper, string) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testDumper := &testDumper{"data"}
	ds := NewDumpServer(testDumper, 10)
	dir, err := os.MkdirTemp("", "testtmp")
	if err != nil {
		t.Fatal(err)
	}

	ds.dumpDir = dir
	ds.dumpPrefix = TestFilePrefix
	return ds, testDumper, dir
}

func mustCreateDump(t *testing.T, ds *DumpServer) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/dumps", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)
	return rr
}

func validateDumpCreated(t *testing.T, expectedNumDumps int, dir string) {
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

	if filesFound != expectedNumDumps {
		t.Errorf("dump server created expected to create %v dump but got %v", expectedNumDumps, filesFound)
	}
}

func validateGetDumps(t *testing.T, ds *DumpServer, expectedNumDumps int) map[string]*Dump {
	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/dumps", nil)
	if err != nil {
		t.Fatal(err)
	}

	ds.mux.ServeHTTP(rr, request)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusCreated)
	}

	dumps := make(map[string]*Dump)

	responseBody, err := io.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(responseBody, &dumps)

	if dumpsLen := len(dumps); dumpsLen != expectedNumDumps {
		t.Errorf("unexpected dumps found, expected:%v  got: %v", expectedNumDumps, dumpsLen)
	}
	return dumps
}

func validateGetDump(t *testing.T, ds *DumpServer, dumpName string, expectedData []byte) {
	rr := httptest.NewRecorder()

	request, err := http.NewRequest("GET", "/dumps/"+dumpName, nil)
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
		t.Errorf("data expected in dump: %s , data found: %s", string(expectedData), string(responseBody))
	}
}
