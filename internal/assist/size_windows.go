package assist

import "io/fs"

func onDisk(fi fs.FileInfo) int64 { return fi.Size() }
