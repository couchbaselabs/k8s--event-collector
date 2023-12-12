package dumpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// The Dumper interface provides dumps for DumpServer to manage
type Dumper interface {
	Dump(io.Writer) error
}

var log = logf.Log.WithName("dump-server")

var tsFormat = "20060102T150405"

// DumpStatus represents the status of a buffer Dump
type DumpStatus string

const (
	// DumpStarted status is when the dump has been started but not complete
	DumpStarted DumpStatus = "Started"
	// DumpComplete status is when the dump has succesfully completed
	DumpComplete DumpStatus = "Complete"
	// DumpFailed status is when the dump has failed
	DumpFailed DumpStatus = "Failed"
)

// DumpCompletionFunc takes a certain action based on the event that triggered it
type DumpCompletionFunc func(d *Dump)

// Dump is a dump of the event buffer
type Dump struct {
	Status DumpStatus
	Name   string
}

// DumpServer serves an API to trigger and fetch dumps
type DumpServer struct {
	mux    *http.ServeMux
	dumper Dumper
	dumps  map[string]*Dump

	// These are callbacks used to trigger notifications when dumps are complete
	dumpCompleteCallbacks []DumpCompletionFunc

	dumpDir string

	dumpPrefix string
}

const dumpDir = "/tmp/"
const dumpFileExtension = ".json"
const dumpPrefix = "event-log-"

// NewDumpServer creates a new DumpServer
func NewDumpServer(dumper Dumper) *DumpServer {
	dm := DumpServer{
		mux:        http.NewServeMux(),
		dumper:     dumper,
		dumps:      make(map[string]*Dump),
		dumpDir:    dumpDir,
		dumpPrefix: dumpPrefix,
	}

	dm.loadExistingFileDumps()

	dm.mux.HandleFunc("/dumps", dm.handleDumps)
	dm.mux.HandleFunc("/dumps/", dm.handleGetDump)
	dm.mux.HandleFunc("/buffer", dm.handleGetBuffer)
	return &dm
}

// AddCompletionCallback adds a completion callback
func (dm *DumpServer) AddCompletionCallback(callback DumpCompletionFunc) {
	dm.dumpCompleteCallbacks = append(dm.dumpCompleteCallbacks, callback)
}

func (dm *DumpServer) loadExistingFileDumps() {
	dirs, err := os.ReadDir(dm.dumpDir)

	if err != nil {
		log.Error(err, "Couldn't read dump directory, no existing dumps loaded")
	}

	for _, d := range dirs {
		if !d.IsDir() && strings.HasPrefix(d.Name(), dm.dumpPrefix) {
			dm.dumps[d.Name()] = &Dump{Status: DumpComplete, Name: d.Name()}
		}
	}
}

func (dm *DumpServer) handleDumps(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dm.handleGetDumps(rw, r)
		return
	case http.MethodPost:
		dm.handlePostDumps(rw, r)
		return
	}
}

func (dm *DumpServer) handleGetDumps(rw http.ResponseWriter, r *http.Request) {
	json.NewEncoder(rw).Encode(dm.dumps)
}

func (dm *DumpServer) handlePostDumps(rw http.ResponseWriter, r *http.Request) {
	dumpName := dm.dumpPrefix + time.Now().Format(tsFormat)

	if _, exists := dm.dumps[dumpName]; exists {
		log.Error(fmt.Errorf("Dump %s already exists", dumpName), "Dump creation failed")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Info("Creating events dump", "dump-name", dumpName)

	rw.WriteHeader(http.StatusCreated)

	go func() {
		dm.createFileDump(dumpName)
	}()
}

func (dm *DumpServer) handleGetDump(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	dumpName := strings.TrimPrefix(r.URL.Path, "/dumps/")

	if dump, exists := dm.dumps[dumpName]; !exists || dump.Status == DumpFailed {
		rw.WriteHeader(http.StatusNotFound)
		return
	} else if dump.Status == DumpStarted {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	} else if dump.Status != DumpComplete {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	filePath := dm.getDumpLocation(dumpName)
	rw.Header().Set("Content-Type", "application/json")
	http.ServeFile(rw, r, filePath)
}

func (dm *DumpServer) createFileDump(dumpName string) {
	log.Info("Creating event dump", "dump-name", dumpName)

	d := &Dump{
		Status: DumpStarted,
		Name:   dumpName,
	}

	dm.dumps[dumpName] = d

	dumpStatus := DumpFailed

	// After we're finished whether succesful or not, update the dump status
	defer func() {
		dm.dumps[dumpName].Status = dumpStatus
	}()

	filePath := filepath.Join(dm.dumpDir, dumpName) + dumpFileExtension
	f, err := os.Create(filePath)

	if err != nil {
		log.Error(err, "Failed to create dump file")
		return
	}

	defer f.Close()

	err = dm.dumper.Dump(f)

	if err != nil {
		log.Error(err, "Error writing dump to file")
		return
	}

	dumpStatus = DumpComplete

	log.Info("Executing Complete Functions", "dump-name", dumpName)
	dm.execDumpCompleteFuncs(d)
}

func (dm *DumpServer) handleGetBuffer(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err := dm.dumper.Dump(rw)

	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
	}
}

// CreateBufferDump creates a dump of the buffer
func (dm *DumpServer) CreateBufferDump() {
	dumpName := dm.dumpPrefix + time.Now().Format(tsFormat)

	if _, exists := dm.dumps[dumpName]; exists {
		log.Error(fmt.Errorf("Dump %s already exists", dumpName), "Dump creation failed")
		return
	}

	log.Info("Creating events dump", "dump-name", dumpName)

	dm.createFileDump(dumpName)
}

func (dm *DumpServer) execDumpCompleteFuncs(d *Dump) {
	for _, callback := range dm.dumpCompleteCallbacks {
		callback(d)
	}
}

func (dm *DumpServer) getDumpLocation(dumpName string) string {
	return filepath.Join(dm.dumpDir, dumpName) + dumpFileExtension
}

// Run starts the server
func (dm *DumpServer) Run(port string) {
	log.Info("Starting Dump Server", "port", port)
	http.ListenAndServe(":"+port, dm.mux)
}
