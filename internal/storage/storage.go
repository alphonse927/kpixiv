package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type ImageMeta struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	Title        string    `json:"title"`
	Artist       string    `json:"artist"`
	ArtistID     string    `json:"artist_id"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Rank         int       `json:"rank"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type History struct {
	Current   string    `json:"current"`
	Images    []string  `json:"images"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PaginationState struct {
	Pages map[string]int `json:"pages"`
}

type Storage struct {
	dataDir      string
	downloadDir  string
	downloadPath string
	homeDir      string
	stateDir     string
}

func New(homeDir, downloadPath string) (*Storage, error) {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir() //nolint: errcheck // ignore error
	}

	dataDir := filepath.Join(homeDir, ".local", "share", "kpixiv")
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}

	stateDir := filepath.Join(homeDir, ".local", "state", "kpixiv")
	if err := os.MkdirAll(stateDir, 0750); err != nil {
		return nil, err
	}

	rankingDir := filepath.Join(dataDir, "Ranking")
	if err := os.MkdirAll(rankingDir, 0750); err != nil {
		return nil, err
	}

	downloadDir := downloadPath
	if downloadPath == "" {
		downloadDir = filepath.Join(homeDir, "Pictures", "KPixiv")
	}

	if err := os.MkdirAll(downloadDir, 0750); err != nil {
		return nil, err
	}

	return &Storage{
		dataDir:      dataDir,
		downloadDir:  downloadDir,
		downloadPath: downloadDir,
		homeDir:      homeDir,
		stateDir:     stateDir,
	}, nil
}

func (s *Storage) DataDir() string {
	return s.dataDir
}

func (s *Storage) StateDir() string {
	return s.stateDir
}

func (s *Storage) DownloadDir() string {
	return s.downloadDir
}

func (s *Storage) RankingDir() string {
	return filepath.Join(s.dataDir, "Ranking")
}

func (s *Storage) MetadataPath() string {
	return filepath.Join(s.stateDir, "metadata.json")
}

func (s *Storage) HistoryPath() string {
	return filepath.Join(s.stateDir, "history.json")
}

func (s *Storage) PaginationPath() string {
	return filepath.Join(s.stateDir, "pagination.json")
}

func (s *Storage) LoadPaginationState() (*PaginationState, error) {
	path := s.PaginationPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PaginationState{Pages: map[string]int{}}, nil
		}
		return nil, err
	}

	var state PaginationState
	if err = json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if state.Pages == nil {
		state.Pages = map[string]int{}
	}

	return &state, nil
}

func (s *Storage) SavePaginationState(state *PaginationState) error {
	if state.Pages == nil {
		state.Pages = map[string]int{}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.PaginationPath(), data, 0600)
}

func (s *Storage) GetRankingPage(key string) (int, error) {
	state, err := s.LoadPaginationState()
	if err != nil {
		return 1, err
	}

	page, ok := state.Pages[key]
	if !ok || page < 1 {
		return 1, nil
	}

	return page, nil
}

func (s *Storage) SetRankingPage(key string, page int) error {
	if page < 1 {
		page = 1
	}

	state, err := s.LoadPaginationState()
	if err != nil {
		return err
	}

	state.Pages[key] = page
	return s.SavePaginationState(state)
}

func (s *Storage) LoadMetadata() (map[string]*ImageMeta, error) {
	path := s.MetadataPath()
	data, rfErr := os.ReadFile(path)
	if rfErr != nil {
		if os.IsNotExist(rfErr) {
			return make(map[string]*ImageMeta), nil
		}
		return nil, rfErr
	}

	var images map[string]*ImageMeta
	if err := json.Unmarshal(data, &images); err != nil {
		return nil, err
	}

	return images, nil
}

func (s *Storage) SaveMetadata(images map[string]*ImageMeta) error {
	data, err := json.MarshalIndent(images, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.MetadataPath(), data, 0600)
}

func (s *Storage) AddImage(meta *ImageMeta) error {
	images, err := s.LoadMetadata()
	if err != nil {
		return err
	}

	images[meta.ID] = meta
	return s.SaveMetadata(images)
}

func (s *Storage) HasImage(id string) (bool, error) {
	images, err := s.LoadMetadata()
	if err != nil {
		return false, err
	}

	_, exists := images[id]
	return exists, nil
}

func (s *Storage) GetImagePath(id string) (string, bool) {
	images, err := s.LoadMetadata()
	if err != nil {
		return "", false
	}

	meta, exists := images[id]
	if exists {
		return meta.Path, true
	}

	// Falling back to the ranking directory
	return s.findImageInRankingDir(id)
}

func (s *Storage) LoadHistory() (*History, error) {
	path := s.HistoryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &History{
				Images:    []string{},
				UpdatedAt: time.Now(),
			}, nil
		}
		return nil, err
	}

	var history History
	if err = json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	return &history, nil
}

func (s *Storage) SaveHistory(history *History) error {
	history.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.HistoryPath(), data, 0600)
}

func (s *Storage) AddToHistory(imageID string) error {
	history, err := s.LoadHistory()
	if err != nil {
		return err
	}

	history.Images = append(history.Images, imageID)
	history.Current = imageID

	if len(history.Images) > 50 {
		history.Images = history.Images[len(history.Images)-50:]
	}

	return s.SaveHistory(history)
}

func (s *Storage) GetCurrentWallpaper() (string, error) {
	history, err := s.LoadHistory()
	if err != nil {
		return "", err
	}

	return history.Current, nil
}

func (s *Storage) GetNextWallpaper() (string, error) {
	history, err := s.LoadHistory()
	if err != nil {
		return "", err
	}

	currentIdx := -1
	for i, img := range history.Images {
		if img == history.Current {
			currentIdx = i
			break
		}
	}

	if currentIdx < 0 || currentIdx >= len(history.Images)-1 {
		if len(history.Images) > 0 {
			return history.Images[0], nil
		}
		return "", nil
	}

	return history.Images[currentIdx+1], nil
}

func (s *Storage) findImageInRankingDir(id string) (string, bool) {
	rankingDir := s.RankingDir()

	var foundPath string
	err := filepath.Walk(rankingDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		// Check if filename contains the image ID
		filename := filepath.Base(path)
		if filepath.Ext(filename) != "" {
			nameWithoutExt := filename[:len(filename)-len(filepath.Ext(filename))]
			if nameWithoutExt == id || filepath.Base(filepath.Dir(path)) == id {
				foundPath = path
				return filepath.SkipAll
			}
		}

		return nil
	})

	if err != nil || foundPath == "" {
		return "", false
	}

	return foundPath, true
}
