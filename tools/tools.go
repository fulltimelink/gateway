package tools

import "os"

// XlGetOsEnv --  @# 获取系统变更 ，可设置默认值
func XlGetOsEnv(key string, def string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	return v
}
