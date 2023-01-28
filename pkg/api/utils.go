package api

import (
	"os"
	"strings"

	"github.com/hashicorp/nomad/client/taskenv"
	"github.com/hashicorp/nomad/helper"
)

func metaKeys(key string) (string, string) {
	cleank := helper.CleanEnvVar(key, '_')
	k := taskenv.MetaPrefix + cleank
	return k, strings.ToUpper(k)
}

func metaGetter(key string, metas ...map[string]string) func(string) string {
	meta := make(map[string]string)

	for _, m := range metas {
		for k, v := range m {
			mk, capitalmk := metaKeys(k)
			meta[mk] = v
			meta[capitalmk] = v
		}
	}

	return func(s string) string {
		return meta[s]
	}
}

func expandMeta(meta map[string]string, parentMetas ...map[string]string) map[string]string {
	expandedMeta := make(map[string]string)

	combined := make([]map[string]string, 0)
	combined = append(combined, meta)
	combined = append(combined, parentMetas...)

	for k, v := range meta {
		expandedMeta[k] = os.Expand(v, metaGetter(k, combined...))
	}

	return expandedMeta
}
