package storage

// Storage interface for persisting case status
type Storage interface {
	Load() (map[string]interface{}, error)
	Save(data map[string]interface{}) error
}

// FileStorage implements Storage using a JSON file
type FileStorage struct {
	filePath string
}

// NewFileStorage creates a new file-based storage
func NewFileStorage(filePath string) *FileStorage {
	return &FileStorage{
		filePath: filePath,
	}
}

// Load loads the previous state from file
func (f *FileStorage) Load() (map[string]interface{}, error) {
	// TODO: implement state loading
	return nil, nil
}

// Save saves the current state to file
func (f *FileStorage) Save(data map[string]interface{}) error {
	// TODO: implement state saving
	return nil
}
