package filesystem

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TristanShz/flow/internal/application"
	"github.com/TristanShz/flow/internal/domain/session"
	"github.com/TristanShz/flow/pkg/timerange"
)

type Sessions []session.Session

func (s Sessions) Len() int {
	return len(s)
}

func (s Sessions) Less(i, j int) bool {
	return s[i].StartTime.Before(s[j].StartTime)
}

func (s Sessions) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type FileSystemSessionRepository struct {
	FlowFolderPath string
}

func NewFileSystemSessionRepository(flowFolderPath string) FileSystemSessionRepository {
	if _, err := os.Stat(flowFolderPath); os.IsNotExist(err) {
		if err := os.MkdirAll(flowFolderPath, 0777); err != nil {
			log.Fatal("Error while creating .flow folder : ", err)
		}
	}

	return FileSystemSessionRepository{
		FlowFolderPath: flowFolderPath,
	}
}

func NotFoundError(id string) error {
	return errors.New("session with id " + id + " not found")
}

type SessionFilename struct {
	StartTime time.Time
	Id        string
	Project   string
}

func (s *SessionFilename) StrippedProject() string {
	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	return reg.ReplaceAllString(s.Project, "")
}

func (s *SessionFilename) String() string {
	return s.Id + "-" + s.StrippedProject() + "-" + strconv.FormatInt(s.StartTime.Unix(), 10) + ".json"
}

func (r *FileSystemSessionRepository) getSessionFileName(s session.Session) string {
	sessionFilename := SessionFilename{
		Id:        s.Id,
		Project:   s.Project,
		StartTime: s.StartTime,
	}

	return sessionFilename.String()
}

func (r *FileSystemSessionRepository) parseSessionFileName(fileName string) (SessionFilename, error) {
	parts := strings.Split(fileName, "-")
	if len(parts) != 3 {
		return SessionFilename{}, errors.New("invalid session file name")
	}
	id := parts[0]
	project := parts[1]
	startTimeUnix, err := strconv.ParseInt(strings.TrimSuffix(parts[2], ".json"), 10, 64)
	if err != nil {
		return SessionFilename{}, err
	}
	return SessionFilename{
		Id:        id,
		Project:   project,
		StartTime: time.Unix(startTimeUnix, 0),
	}, nil
}

func (r *FileSystemSessionRepository) readFlowFolder() ([]fs.FileInfo, error) {
	dir, err := os.Open(r.FlowFolderPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	fileInfos, err := dir.Readdir(-1)
	if err != nil {
		return nil, err
	}

	return fileInfos, nil
}

func (r *FileSystemSessionRepository) FindById(id string) *session.Session {
	fileInfos, err := r.readFlowFolder()
	if err != nil {
		log.Fatal(err)
	}

	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			continue
		}

		sessionFilename, err := r.parseSessionFileName(fileInfo.Name())
		if err != nil {
			log.Fatalf("error while parsing file name %v : '%v'", fileInfo.Name(), err)
		}

		if sessionFilename.Id == id {
			filePath := filepath.Join(r.FlowFolderPath, fileInfo.Name())
			file, err := os.ReadFile(filePath)
			if err != nil {
				log.Fatalf("Error while reading file %v : '%v'", fileInfo.Name(), err)
			}

			session, convertErr := r.rawFileToSession(file)
			if convertErr != nil {
				log.Fatalf("Invalid session data for file : %v", fileInfo.Name())
			}

			return session
		}
	}

	return nil
}

func (r *FileSystemSessionRepository) Save(sessionToSave session.Session) error {
	marshaled, marshaledErr := json.MarshalIndent(sessionToSave, "", "  ")

	if marshaledErr != nil {
		return marshaledErr
	}

	fullPath := filepath.Join(r.FlowFolderPath, r.getSessionFileName(sessionToSave))
	saveErr := os.WriteFile(fullPath, marshaled, 0666)

	if saveErr != nil {
		return saveErr
	}

	return nil
}

func (r *FileSystemSessionRepository) Delete(id string) error {
	fileInfos, err := r.readFlowFolder()
	if err != nil {
		log.Fatal(err)
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			continue
		}

		filenameInfo, err := r.parseSessionFileName(fileInfo.Name())
		if err != nil {
			log.Fatalf("error while parsing file name %v : '%v'", fileInfo.Name(), err)
		}
		if filenameInfo.Id == id {
			filepath := filepath.Join(r.FlowFolderPath, fileInfo.Name())
			deleteErr := os.Remove(filepath)
			if deleteErr != nil {
				log.Fatalf("error while deleting file %v : '%v'", fileInfo.Name(), deleteErr)
			}
			return nil
		}
	}

	return NotFoundError(id)
}

func (r *FileSystemSessionRepository) rawFileToSession(raw []byte) (*session.Session, error) {
	var sessionData session.Session
	if err := json.Unmarshal(raw, &sessionData); err != nil {
		return nil, err
	}

	return &sessionData, nil
}

func (r *FileSystemSessionRepository) FindAllSessions(filters *application.SessionsFilters) []session.Session {
	fileInfos, err := r.readFlowFolder()
	if err != nil {
		log.Fatal(err)
	}

	if filters != nil {
		if !filters.Timerange.IsZero() {
			fileInfos = r.filterByTimeRange(fileInfos, filters.Timerange)
		}

		if filters.Project != "" {
			fileInfos = r.filterByProject(fileInfos, filters.Project)
		}
	}

	sessions := Sessions{}

	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			continue
		}

		filePath := filepath.Join(r.FlowFolderPath, fileInfo.Name())
		file, err := os.ReadFile(filePath)
		if err != nil {
			log.Fatalf("error while reading file %v : '%v'", fileInfo.Name(), err)
		}

		session, convertErr := r.rawFileToSession(file)
		if convertErr != nil {
			log.Fatalf("invalid session data for file : %v", fileInfo.Name())
		}
		sessions = append(sessions, *session)
	}

	sort.Sort(sessions)

	return sessions
}

func (r *FileSystemSessionRepository) filterByProject(fileInfos []fs.FileInfo, project string) []fs.FileInfo {
	filteredFileInfos := []fs.FileInfo{}
	for _, fileInfo := range fileInfos {
		sessionFilename, err := r.parseSessionFileName(fileInfo.Name())
		if err != nil {
			log.Fatalf("error while parsing file name %v : '%v'", fileInfo.Name(), err)
		}
		if sessionFilename.Project == project {
			filteredFileInfos = append(filteredFileInfos, fileInfo)
		}
	}
	return filteredFileInfos
}

func (r *FileSystemSessionRepository) filterByTimeRange(fileInfos []fs.FileInfo, timeRange timerange.TimeRange) []fs.FileInfo {
	filteredFileInfos := []fs.FileInfo{}
	for _, fileInfo := range fileInfos {
		sessionFilename, err := r.parseSessionFileName(fileInfo.Name())
		if err != nil {
			log.Fatalf("error while parsing file name %v : '%v'", fileInfo.Name(), err)
		}
		if timeRange.JustUntil() {
			if sessionFilename.StartTime.Before(timeRange.Until) {
				filteredFileInfos = append(filteredFileInfos, fileInfo)
			}
		} else if timeRange.JustSince() {
			if sessionFilename.StartTime.After(timeRange.Since) {
				filteredFileInfos = append(filteredFileInfos, fileInfo)
			}
		} else if timeRange.SinceAndUntil() {
			if sessionFilename.StartTime.After(timeRange.Since) && sessionFilename.StartTime.Before(timeRange.Until) {
				filteredFileInfos = append(filteredFileInfos, fileInfo)
			}
		} else {
			filteredFileInfos = append(filteredFileInfos, fileInfo)
		}
	}
	return filteredFileInfos
}

func (r *FileSystemSessionRepository) FindLastSession() *session.Session {
	fileInfos, err := r.readFlowFolder()
	if err != nil {
		log.Fatal(err)
	}

	fileNames := []SessionFilename{}

	for _, fileInfo := range fileInfos {
		if !fileInfo.IsDir() {
			filenameInfo, err := r.parseSessionFileName(fileInfo.Name())
			if err != nil {
				log.Fatalf("error while parsing file name %v : '%v'", fileInfo.Name(), err)
			}
			fileNames = append(fileNames, filenameInfo)
		}
	}

	if len(fileNames) == 0 {
		return nil
	}

	sort.Slice(fileNames, func(i, j int) bool {
		return fileNames[j].StartTime.Before(fileNames[i].StartTime)
	})

	lastSessionFile := fileNames[0].String()

	lastSessionFilePath := filepath.Join(r.FlowFolderPath, lastSessionFile)

	fileData, err := os.ReadFile(lastSessionFilePath)
	if err != nil {
		log.Fatalf("error while reading file %v", lastSessionFilePath)
	}

	session, convertErr := r.rawFileToSession(fileData)

	if convertErr != nil {
		log.Fatalf("invalid session data for file : %v", lastSessionFilePath)
	}

	return session
}

func (r *FileSystemSessionRepository) FindAllProjects() []string {
	sessions := r.FindAllSessions(nil)

	projects := []string{}

	for _, session := range sessions {
		if slices.Contains(projects, session.Project) {
			continue
		}

		projects = append(projects, session.Project)
	}

	return projects
}

func (r *FileSystemSessionRepository) FindAllProjectTags(project string) []string {
	sessionsForProject := r.FindAllSessions(&application.SessionsFilters{Project: project})

	tags := []string{}

	for _, session := range sessionsForProject {
		for _, tag := range session.Tags {
			if slices.Contains(tags, tag) {
				continue
			}

			tags = append(tags, tag)
		}
	}

	return tags
}
