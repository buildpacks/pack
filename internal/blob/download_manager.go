package blob

import (
	"context"
	"fmt"
	"strings"
)

type Downloader interface {
	Download(ctx context.Context, pathOrURI string, options ...DownloadOption) (Blob, error)
}

type DownloadManager struct {
	downloader  Downloader
	workerCount int
}

func NewDownloadManager(d Downloader, workerCount int) DownloadManager {
	return DownloadManager{
		downloader:  d,
		workerCount: workerCount,
	}
}

type DownloadJob struct {
	URI    string
	Sha256 string
}

type DownloadResult struct {
	Blob Blob
	Err  error
	DownloadJob
}

// TODO -Dan- parallel downloads should cleanly exit with Ctrl-C
// TODO -Dan- parallel download output is a bit messed up.
// existing behavior is a bit dangerous and can poison the cache.
func (dm *DownloadManager) DownloadAndValidate(jobs ...DownloadJob) (map[DownloadJob]DownloadResult, error) {
	resultMap := make(map[DownloadJob]DownloadResult)
	results := make(chan DownloadResult, len(jobs))
	jobQueue := make(chan DownloadJob, len(jobs))

	for workerCount := 0; workerCount < dm.workerCount; workerCount++ {
		go downloadWorker(dm.downloader, jobQueue, results)
	}

	for _, job := range jobs {
		jobQueue <- job
	}
	close(jobQueue)

	errors := []error{}
	for i := 0; i < len(jobs); i++ {
		r := <-results
		switch r.Err {
		case nil:
			resultMap[r.DownloadJob] = r
		default:
			errors = append(errors, r.Err)
		}
	}

	joinedErrs := fmt.Errorf("the following errors occurred during download: %q", errorJoin(errors, ", "))
	if len(errors) > 0 {
		return resultMap, joinedErrs
	}
	return resultMap, nil
}

func errorJoin(elems []error, sep string) string {
	strArr := []string{}
	for _, elem := range elems {
		strArr = append(strArr, elem.Error())
	}

	return strings.Join(strArr, sep)
}

func downloadWorker(downloader Downloader, jobs <-chan DownloadJob, results chan<- DownloadResult) {
	for j := range jobs {
		var b Blob = nil
		var err error
		if j.URI != "" {
			b, err = downloader.Download(context.Background(), j.URI, RawDownload, ValidateDownload(j.Sha256))
		}
		results <- DownloadResult{
			Blob:        b,
			Err:         err,
			DownloadJob: j,
		}
	}
}
