package tarball

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func CombineTarballs(readers []io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(buf)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, reader := range readers {
		err := func() error {
			gzipReader, err := gzip.NewReader(reader)
			if err != nil {
				return err
			}
			defer gzipReader.Close()

			tarReader := tar.NewReader(gzipReader)
			for {
				header, err := tarReader.Next()
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}

				if err = tarWriter.WriteHeader(header); err != nil {
					return err
				}

				if _, err = io.Copy(tarWriter, tarReader); err != nil {
					return err
				}
			}
		}()
		if err != nil {
			return nil, err
		}
	}

	tarWriter.Close()
	gzipWriter.Close()
	return buf.Bytes(), nil
}

// Create a gzipped tarball of a given path (regular file or directory).
// If removeFiles is true, this method removes the given files after the tarball
// is created.
func CreateGzippedTarball(tarFilePath string, path string, removeFiles bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to get file info of %s: %s", path, err)
	}

	tarballDir := filepath.Dir(tarFilePath)
	if err := os.MkdirAll(tarballDir, 0700); err != nil {
		return fmt.Errorf("failed to create parent paths for %s: %s",
			tarFilePath, err)
	}

	file, err := os.Create(tarFilePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %s", tarFilePath, err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	var filePaths []string
	if !info.IsDir() {
		filePaths = []string{path}
	} else {
		filePaths, err = filepath.Glob(path + "/*")
		if err != nil {
			return fmt.Errorf("failed to get all files under %s: %s", path, err)
		}
	}

	dirOfPath := filepath.Dir(path)
	for _, filePath := range filePaths {
		if filePath == tarFilePath {
			continue
		}

		err := addFileToTarWriter(filePath, dirOfPath, tarWriter)
		if err != nil {
			return fmt.Errorf("failed to add %s to tar writer: %s", filePath, err)
		}

		if removeFiles {
			os.Remove(filePath)
		}
	}
	return nil
}

// Get all file paths in the tarball that match the given regex.
func GetFilePathsWithRegex(tarFilePath, regex string) ([]string, error) {
	file, err := os.Open(tarFilePath)
	if err != nil {
		return nil, err
	}

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}

	var files []string
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				return files, nil
			}
			return files, err
		}

		r := regexp.MustCompile(regex)
		if r.MatchString(header.Name) {
			files = append(files, header.Name)
		}
	}
}

func ReadFileFromGzippedTarball(data []byte, path string) ([]byte, error) {
	buf := bytes.NewBuffer(data)
	gzipReader, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				err = fmt.Errorf("no file named %s in tarball", path)
			}
			return nil, err
		}
		if header.Name == path {
			return ioutil.ReadAll(tarReader)
		}
	}
}

func WriteTarballToTarWriter(data []byte, tarWriter *tar.Writer, pathPrefixToTrim string) error {
	gzipReader, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if pathPrefixToTrim != "" {
			header.Name = strings.TrimPrefix(header.Name, pathPrefixToTrim)
		}

		if err = tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if _, err = io.Copy(tarWriter, tarReader); err != nil {
			return err
		}
	}
}

func addFileToTarWriter(filePath, dirPath string, tarWriter *tar.Writer) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	relFilePath, err := filepath.Rel(dirPath, filePath)
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:    relFilePath,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}

	if err = tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tarWriter, file)
	return err
}
