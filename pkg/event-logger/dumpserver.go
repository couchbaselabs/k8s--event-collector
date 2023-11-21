package elogger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/couchbase/couchbase-operator/pkg/info/util"
)

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

// Dump is a dump of the event buffer
type Dump struct {
	Status DumpStatus
}

// DumpServer serves an API to trigger and fetch dumps
type DumpServer struct {
	mux    *http.ServeMux
	logger *EventLogger
	dumps  map[string]*Dump
}

const dumpDir = "/tmp/"
const dumpFileExtension = ".json"
const dumpPrefix = "event-log-"

// NewDumpServer creates a new DumpServer
func NewDumpServer(logger *EventLogger) *DumpServer {
	dm := DumpServer{
		mux:    http.NewServeMux(),
		logger: logger,
		dumps:  make(map[string]*Dump),
	}

	dm.loadExistingFileDumps()

	dm.mux.HandleFunc("/dumps", dm.handleDumps)
	dm.mux.HandleFunc("/dumps/", dm.handleGetDump)
	dm.mux.HandleFunc("/buffer", dm.handleGetBuffer)
	return &dm
}

func (dm *DumpServer) loadExistingFileDumps() {
	dirs, err := os.ReadDir(dumpDir)

	if err != nil {
		log.Error(err, "Couldn't read dump directory, no existing dumps loaded")
	}

	for _, d := range dirs {
		if d.IsDir() {
			continue
		}

		if strings.HasPrefix(d.Name(), dumpPrefix) {
			dm.dumps[d.Name()] = &Dump{DumpComplete}
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
	dumpName := dumpPrefix + util.Timestamp()

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

	filePath := dm.logger.getDumpLocation(dumpName)
	_, filename := filepath.Split(filePath)

	rw.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filename))
	rw.Header().Set("Content-Type", "application/json")
	http.ServeFile(rw, r, filePath)
}

func (dm *DumpServer) createFileDump(dumpName string) {
	log.Info("Creating event dump", "dump-name", dumpName)

	dm.dumps[dumpName] = &Dump{
		Status: DumpStarted,
	}

	dumpStatus := DumpFailed

	// After we're finished whether succesful or not, update the dump status
	defer func() {
		dm.dumps[dumpName].Status = dumpStatus
	}()

	f, err := os.Create(dumpDir + dumpName + dumpFileExtension)

	if err != nil {
		log.Error(err, "Failed to create dump file")
		return
	}

	defer f.Close()

	err = dm.logger.dump(f)

	if err != nil {
		log.Error(err, "Error writing dump to file")
		return
	}

	dumpStatus = DumpComplete
}

func (dm *DumpServer) handleGetBuffer(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	rw.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(dumpPrefix+util.Timestamp()+dumpFileExtension))
	rw.Header().Set("Content-Type", "application/json")
	err := dm.logger.dump(rw)

	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
	}
}

// CreateBufferDump creates a dump of the buffer
func (dm *DumpServer) CreateBufferDump() {
	dumpName := dumpPrefix + util.Timestamp()

	if _, exists := dm.dumps[dumpName]; exists {
		log.Error(fmt.Errorf("Dump %s already exists", dumpName), "Dump creation failed")
		return
	}

	log.Info("Creating events dump", "dump-name", dumpName)

	dm.createFileDump(dumpName)
}

// Run starts the server
func (dm *DumpServer) Run(port string) {
	log.Info("Starting Dump Server 2", "port", port)
	http.ListenAndServe(":"+port, dm.mux)
}
