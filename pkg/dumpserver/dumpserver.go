package dumpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
	mux        *http.ServeMux
	dumper     Dumper
	dumps      map[string]*Dump
	dumpsMutex sync.RWMutex

	// These are callbacks used to trigger notifications when dumps are complete
	dumpCompleteCallbacks []DumpCompletionFunc

	dumpDir    string
	dumpPrefix string

	maxDumps int
}

const dumpDir = "/tmp/"
const dumpFileExtension = ".json"
const dumpPrefix = "event-log-"

// NewDumpServer creates a new DumpServer
func NewDumpServer(dumper Dumper, maxDumps int) *DumpServer {
	dm := DumpServer{
		mux:        http.NewServeMux(),
		dumper:     dumper,
		dumps:      make(map[string]*Dump),
		dumpDir:    dumpDir,
		dumpPrefix: dumpPrefix,
		maxDumps:   maxDumps,
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
	dm.dumpsMutex.Lock()
	defer dm.dumpsMutex.Unlock()
	dirs, err := os.ReadDir(dm.dumpDir)

	if err != nil {
		log.Error(err, "Couldn't read dump directory, no existing dumps loaded")
	}

	for _, d := range dirs {
		if !d.IsDir() && strings.HasPrefix(d.Name(), dm.dumpPrefix) {
			dumpeName, _ := strings.CutSuffix(d.Name(), dumpFileExtension)
			dm.dumps[dumpeName] = &Dump{Status: DumpComplete, Name: dumpeName}
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
	dm.dumpsMutex.RLock()
	defer dm.dumpsMutex.RUnlock()
	json.NewEncoder(rw).Encode(dm.dumps)
}

func (dm *DumpServer) handlePostDumps(rw http.ResponseWriter, r *http.Request) {
	dumpName := dm.dumpPrefix + time.Now().Format(tsFormat)

	dm.purgeOldDumps(dm.maxDumps - 1)

	if err := dm.createFileDump(dumpName); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	rw.WriteHeader(http.StatusCreated)

	rw.Write([]byte(dumpName))
}

func (dm *DumpServer) purgeOldDumps(maxDumps int) {
	dm.dumpsMutex.Lock()
	defer dm.dumpsMutex.Unlock()
	if len(dm.dumps) <= maxDumps {
		return
	}

	dumpNames := make([]string, len(dm.dumps))

	i := 0
	for dump := range dm.dumps {
		dumpNames[i] = dump
		i++
	}

	slices.Sort(dumpNames)

	dumpsToDelete := len(dm.dumps) - maxDumps

	for i = 0; i < dumpsToDelete; i++ {
		dumpName := dumpNames[i]
		log.Info("Removing old dump", "dumpName", dumpName)
		dumpLocation := dm.getDumpLocation(dumpName)
		os.Remove(dumpLocation)
		delete(dm.dumps, dumpName)
	}
}

func (dm *DumpServer) handleGetDump(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	dumpName := strings.TrimPrefix(r.URL.Path, "/dumps/")

	dm.dumpsMutex.RLock()
	defer dm.dumpsMutex.RUnlock()
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

func (dm *DumpServer) createFileDump(dumpName string) error {
	dm.dumpsMutex.Lock()
	defer dm.dumpsMutex.Unlock()
	log.Info("Creating event dump", "dump-name", dumpName)

	d := &Dump{
		Status: DumpStarted,
		Name:   dumpName,
	}

	if _, exists := dm.dumps[dumpName]; exists {
		err := fmt.Errorf("Dump %s already exists", dumpName)
		log.Error(err, "Dump creation failed")
		return err
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
		return err
	}

	defer f.Close()

	err = dm.dumper.Dump(f)

	if err != nil {
		log.Error(err, "Error writing dump to file")
		return err
	}

	dumpStatus = DumpComplete

	log.Info("Executing Complete Functions", "dump-name", dumpName)
	go dm.execDumpCompleteFuncs(d)
	return nil
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
