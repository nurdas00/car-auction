package httpapi

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// noDirListingFS оборачивает http.Dir и прячет каталоги за 404: без этого
// http.FileServer генерирует HTML-листинг всех файлов, что позволяет
// перечислить вообще все загруженные фото/видео (включая приватные —
// черновики/на модерации/отклонённые лоты) без какой-либо авторизации
// (см. code review). Сами файлы остаются публичными по случайному
// UUID-имени, как и раньше.
type noDirListingFS struct {
	inner http.FileSystem
}

func (fs noDirListingFS) Open(name string) (http.File, error) {
	f, err := fs.inner.Open(name)
	if err != nil {
		return nil, err
	}
	if stat, err := f.Stat(); err == nil && stat.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

// maxUploadBytes — потолок на один файл (с запасом под короткое видео).
// В проде стоило бы разделить лимиты фото/видео и добавить реальную валидацию
// по содержимому (magic bytes), а не только по расширению — для MVP с локальным
// диском этого достаточно: хранится файл под случайным UUID-именем (обход пути
// исключён), а Content-Type при отдаче назначается по расширению, так что
// подменённое содержимое не исполнится как HTML/JS в браузере.
const maxUploadBytes = 100 << 20 // 100MB

var allowedUploadExt = map[string]string{
	".jpg":  "image",
	".jpeg": "image",
	".png":  "image",
	".webp": "image",
	".mp4":  "video",
	".webm": "video",
	".mov":  "video",
}

// handleUpload принимает один файл (multipart/form-data, поле "file") и
// возвращает публичный URL для его дальнейшей подстановки в photos/videos
// при создании или редактировании лота (ТЗ-дополнение: загрузка медиа).
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if s.UploadsDir == "" {
		writeError(w, http.StatusInternalServerError, "uploads are not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	kind, ok := allowedUploadExt[ext]
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported file type")
		return
	}

	name := uuid.New().String() + ext
	dst, err := os.OpenFile(filepath.Join(s.UploadsDir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save file")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"url":  "/uploads/" + name,
		"kind": kind,
	})
}
