package layer

import (
	iarchive "github.com/buildpacks/imgutil/archive"

	"github.com/buildpacks/pack/internal/archive"
)

func CreateSingleFileTar(tarFile, path, txt string, twf iarchive.TarWriterFactory) error {
	tarBuilder := archive.TarBuilder{}
	tarBuilder.AddFile(path, 0644, archive.NormalizedDateTime, []byte(txt))
	return tarBuilder.WriteToPath(tarFile, twf)
}
