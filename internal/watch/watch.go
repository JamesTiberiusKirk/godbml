package watch

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

type EventKind int

const (
	EventDBML EventKind = iota
	EventMeta
)

type Event struct {
	Kind EventKind
	Path string
}

type Watcher struct {
	fsw      *fsnotify.Watcher
	dbmlPath string
	metaPath string
	events   chan Event
	done     chan struct{}
}

func New(dbmlPath, metaPath string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(dbmlPath)
	if err := fsw.Add(dir); err != nil {
		fsw.Close()
		return nil, err
	}
	w := &Watcher{
		fsw:      fsw,
		dbmlPath: dbmlPath,
		metaPath: metaPath,
		events:   make(chan Event, 8),
		done:     make(chan struct{}),
	}
	go w.loop()
	return w, nil
}

func (w *Watcher) Events() <-chan Event { return w.events }

func (w *Watcher) Close() error {
	close(w.done)
	return w.fsw.Close()
}

const debounce = 80 * time.Millisecond

// loop fans fsnotify events into our debounced (DBML, Meta) event stream.
//
// Editors save in different ways: in-place write (vim with :set noatomic),
// tempfile + rename (vim, JetBrains, VS Code), or backup + replace. Some of
// these fire events on the temp path rather than the target file, and some
// trigger CHMOD instead of WRITE. So instead of trusting any single fsnotify
// op, we re-stat both target paths after every event in the watched directory
// and emit on real content/mtime changes.
func (w *Watcher) loop() {
	defer close(w.events)

	var (
		dbmlPending bool
		metaPending bool
		timer       *time.Timer
		timerC      <-chan time.Time

		lastDBML statSnapshot
		lastMeta statSnapshot
	)
	lastDBML = statOf(w.dbmlPath)
	lastMeta = statOf(w.metaPath)

	flush := func() {
		if dbmlPending {
			dbmlPending = false
			select {
			case w.events <- Event{Kind: EventDBML, Path: w.dbmlPath}:
			case <-w.done:
				return
			}
		}
		if metaPending {
			metaPending = false
			select {
			case w.events <- Event{Kind: EventMeta, Path: w.metaPath}:
			case <-w.done:
				return
			}
		}
	}

	checkChanged := func() {
		if cur := statOf(w.dbmlPath); cur.changedFrom(lastDBML) {
			lastDBML = cur
			dbmlPending = true
		}
		if cur := statOf(w.metaPath); cur.changedFrom(lastMeta) {
			lastMeta = cur
			metaPending = true
		}
	}

	scheduleFlush := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
			timerC = timer.C
		} else {
			timer.Reset(debounce)
		}
	}

	for {
		select {
		case <-w.done:
			return
		case _, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			checkChanged()
			if dbmlPending || metaPending {
				scheduleFlush()
			}
		case <-timerC:
			timer = nil
			timerC = nil
			// Re-check on flush in case the final write landed during debounce.
			checkChanged()
			flush()
		case <-w.fsw.Errors:
		}
	}
}

type statSnapshot struct {
	exists  bool
	size    int64
	modTime time.Time
}

func statOf(path string) statSnapshot {
	info, err := os.Stat(path)
	if err != nil {
		return statSnapshot{}
	}
	return statSnapshot{exists: true, size: info.Size(), modTime: info.ModTime()}
}

func (s statSnapshot) changedFrom(prev statSnapshot) bool {
	if s.exists != prev.exists {
		return true
	}
	if !s.exists {
		return false
	}
	return s.size != prev.size || !s.modTime.Equal(prev.modTime)
}
