package store

import (
	"encoding/json"
	"sync"
)

// Exclusion limits tracking for one application.
type Exclusion struct {
	App  string `json:"app"`  // executable name, e.g. "keepass.exe"
	Mode string `json:"mode"` // titles | all | screenshots
}

// Settings are the user-editable preferences.
type Settings struct {
	AwayMinutes        int         `json:"awayMinutes"`
	TreatSoundAsActive bool        `json:"treatSoundAsActive"`
	TrackTitles        bool        `json:"trackTitles"`
	TrackDocuments     bool        `json:"trackDocuments"`
	TrackPrivate       bool        `json:"trackPrivate"`
	ReadAddressBar     bool        `json:"readAddressBar"`
	AwayPrompt         bool        `json:"awayPrompt"`
	AwayPromptMinutes  int         `json:"awayPromptMinutes"`
	LongAwayOff        bool        `json:"longAwayOff"`   // a long time away counts as off (see LongAway)
	LongAwayHours      int         `json:"longAwayHours"` // from this long
	Exclusions         []Exclusion `json:"exclusions"`

	ScheduleOn    bool    `json:"scheduleOn"`
	ScheduleDays  [7]bool `json:"scheduleDays"` // Monday first
	ScheduleStart string  `json:"scheduleStart"`
	ScheduleEnd   string  `json:"scheduleEnd"`

	ShotsOn        bool   `json:"shotsOn"`
	ShotsInterval  int    `json:"shotsInterval"` // minutes
	ShotsMaxWidth  int    `json:"shotsMaxWidth"`
	ShotsQuality   int    `json:"shotsQuality"`
	ShotsKeepDays  int    `json:"shotsKeepDays"`
	ShotsSkipSame  bool   `json:"shotsSkipSame"`
	ShotsAllScreen bool   `json:"shotsAllScreens"`
	ShotsDir       string `json:"shotsDir"` // "" = the screenshots folder in the data folder

	Language       string `json:"language"`   // auto | en | zh
	Theme          string `json:"theme"`      // system | light | dark
	LightTheme     string `json:"lightTheme"` // color theme in light mode: classic, githublight, …
	DarkTheme      string `json:"darkTheme"`  // color theme in dark mode: classic, onedark, …
	DurationFormat string `json:"durationFormat"`
	WeekStart      int    `json:"weekStart"` // 1 = Monday, 0 = Sunday
	Clock24        bool   `json:"clock24"`

	RunAtStartup   bool `json:"runAtStartup"`
	StartMinimized bool `json:"startMinimized"`
	CloseToTray    bool `json:"closeToTray"`
	RecentTags     int  `json:"recentTags"`
	StopwatchNag   int  `json:"stopwatchNag"` // minutes, 0 = off

	BackupDays int `json:"backupDays"` // automatic backup interval, 0 = off
	BackupKeep int `json:"backupKeep"`

	DayGrid         []GridItem `json:"dayGrid"`       // position and size of the day view blocks
	DayGridPinned   bool       `json:"dayGridPinned"` // the day view layout can't be moved or resized
	MergeSubdomains bool       `json:"mergeSubdomains"`
	SiteIcons       bool       `json:"siteIcons"`   // download icons the browsers don't have from the sites
	UpdateCheck     bool       `json:"updateCheck"` // look for new releases now and then
}

// GridItem places one day view block: Y is its row, X and W its columns (of
// 12, fractional) and H the row height in grid units (0 for a row fitting its
// content).
type GridItem struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  int     `json:"y"`
	W  float64 `json:"w"`
	H  float64 `json:"h"`
}

// DefaultSettings mirrors ManicTime's defaults, with screenshots enabled at a
// modest size and interval.
func DefaultSettings() Settings {
	return Settings{
		AwayMinutes: 10, TreatSoundAsActive: true, TrackTitles: true, TrackDocuments: true, TrackPrivate: true,
		ReadAddressBar: true, AwayPrompt: true, AwayPromptMinutes: 10, LongAwayHours: 6, Exclusions: []Exclusion{},
		ScheduleDays: [7]bool{true, true, true, true, true, false, false}, ScheduleStart: "08:00", ScheduleEnd: "18:00",
		ShotsOn: true, ShotsInterval: 5, ShotsMaxWidth: 1280, ShotsQuality: 60, ShotsKeepDays: 30, ShotsSkipSame: true,
		Language: "auto", Theme: "system", LightTheme: "classic", DarkTheme: "classic", DurationFormat: "hm", WeekStart: 1, Clock24: true,
		CloseToTray: true, RecentTags: 10, BackupKeep: 5, MergeSubdomains: true, SiteIcons: true, UpdateCheck: true,
	}
}

var settingsMu sync.RWMutex

// Settings loads settings, filling defaults for missing keys.
func (db *DB) Settings() Settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	s := DefaultSettings()
	if raw := db.Meta("settings"); raw != "" {
		json.Unmarshal([]byte(raw), &s)
	}
	if s.AwayMinutes < 1 {
		s.AwayMinutes = 1
	}
	if s.LongAwayHours < 1 || s.LongAwayHours > 12 {
		s.LongAwayHours = 6
	}
	if s.ShotsInterval < 1 {
		s.ShotsInterval = 1
	}
	if s.Exclusions == nil {
		s.Exclusions = []Exclusion{}
	}
	return s
}

// SaveSettings persists settings.
func (db *DB) SaveSettings(s Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	b, _ := json.Marshal(s)
	if err := db.SetMeta("settings", string(b)); err != nil {
		return err
	}
	if m := s.longAway(); db.longAway.Swap(m) != m {
		db.touch() // the timelines read differently now
	}
	return nil
}

// longAway is how long a time away lasts at least to count as off, in
// seconds; 0 when the option is off.
func (st Settings) longAway() int64 {
	if !st.LongAwayOff {
		return 0
	}
	if st.LongAwayHours < 1 || st.LongAwayHours > 12 {
		st.LongAwayHours = 6
	}
	return int64(st.LongAwayHours) * 3600
}

// LongAway reports whether an away stretch from unix time s to e counts as
// off: with the option on, when it lasts LongAwayHours or more.
func (st Settings) LongAway(s, e int64) bool { return longAway(s, e, st.longAway()) }
