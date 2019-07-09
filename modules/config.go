// Copyright 2019 The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modules

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/gohugoio/hugo/config"
	"github.com/gohugoio/hugo/hugofs/files"
	"github.com/gohugoio/hugo/langs"
	"github.com/mitchellh/mapstructure"
)

type Config struct {
	Mounts  []Mount
	Imports []Import
}

type Import struct {
	Path   string // Module path
	Mounts []Mount
}

type Mount struct {
	// TODO(bep) mod add IgnoreFiles: slice of GOB pattern matching content/*
	Source string // relative path in source repo, e.g. "scss"
	Target string // relative target path, e.g. "assets/bootstrap/scss"

	// TODO(bep) mod
	Lang string
}

func (m Mount) Component() string {
	return strings.Split(m.Target, fileSeparator)[0]
}

// ApplyProjectConfigDefaults applies default/missing module configuration for
// the main project.
func ApplyProjectConfigDefaults(cfg config.Provider, mod Module) error {
	moda := mod.(*moduleAdapter)

	// TODO(bep) mod we need a way to check if contentDir etc. is really set.
	// ... so remove the default settings for these.

	// Map legacy directory config into the new module.
	languages := cfg.Get("languagesSortedDefaultFirst").(langs.Languages)
	isMultiHost := languages.IsMultihost()

	// To bridge between old and new configuration format we need
	// a way to make sure all of the core components are configured on
	// the basic level.
	componentsConfigured := make(map[string]bool)
	for _, mnt := range moda.mounts {
		componentsConfigured[mnt.Component()] = true
	}

	type dirKeyComponent struct {
		key          string
		component    string
		multilingual bool
	}

	dirKeys := []dirKeyComponent{
		{"contentDir", files.ComponentFolderContent, true},
		{"dataDir", files.ComponentFolderData, false},
		{"layoutDir", files.ComponentFolderLayouts, false},
		{"i18nDir", files.ComponentFolderI18n, false},
		{"archetypeDir", files.ComponentFolderArchetypes, false},
		{"assetDir", files.ComponentFolderAssets, false},
		{"resourceDir", files.ComponentFolderResources, false},
		{"", files.ComponentFolderStatic, isMultiHost},
	}

	createMountsFor := func(d dirKeyComponent, cfg config.Provider) []Mount {
		var lang string
		if language, ok := cfg.(*langs.Language); ok {
			lang = language.Lang
		}

		// Static mounts are a little special.
		if d.component == files.ComponentFolderStatic {
			var mounts []Mount
			staticDirs := getStaticDirs(cfg)
			if len(staticDirs) > 0 {
				componentsConfigured[d.component] = true
			}

			for _, dir := range staticDirs {
				mounts = append(mounts, Mount{Lang: lang, Source: dir, Target: d.component})
			}

			return mounts

		}

		if cfg.IsSet(d.key) {
			source := cfg.GetString(d.key)
			componentsConfigured[d.component] = true

			return []Mount{Mount{
				// No lang set for layouts etc.
				Source: source,
				Target: d.component}}
		}

		return nil
	}

	createMounts := func(d dirKeyComponent) []Mount {
		var mounts []Mount
		if d.multilingual {
			if d.component == files.ComponentFolderContent {
				seen := make(map[string]bool)
				for _, language := range languages {
					contentDir := language.ContentDir
					if seen[contentDir] {
						continue
					}
					seen[contentDir] = true
					mounts = append(mounts, Mount{Lang: language.Lang, Source: contentDir, Target: d.component})
				}

				componentsConfigured[d.component] = len(seen) > 0

			} else {
				for _, language := range languages {
					mounts = append(mounts, createMountsFor(d, language)...)
				}
			}
		} else {
			mounts = append(mounts, createMountsFor(d, cfg)...)
		}

		return mounts
	}

	var mounts []Mount
	for _, dirKey := range dirKeys {
		if componentsConfigured[dirKey.component] {

			continue
		}

		mounts = append(mounts, createMounts(dirKey)...)

	}

	// Add default configuration
	for _, dirKey := range dirKeys {
		if componentsConfigured[dirKey.component] {
			continue
		}
		mounts = append(mounts, Mount{Source: dirKey.component, Target: dirKey.component})
	}

	// Remove duplicates
	seen := make(map[string]bool)
	tmp := mounts[:0]
	for _, m := range mounts {
		key := path.Join(m.Lang, m.Source, m.Target)
		if !seen[key] {
			tmp = append(tmp, m)
		}
		seen[key] = true
	}

	moda.mounts = mounts

	return nil
}

// DecodeConfig creates a modules Config from a given Hugo configuration.
func DecodeConfig(cfg config.Provider) (c Config, err error) {
	if cfg == nil {
		return
	}

	themeSet := cfg.IsSet("theme")
	moduleSet := cfg.IsSet("module")

	if moduleSet {
		m := cfg.GetStringMap("module")
		err = mapstructure.WeakDecode(m, &c)

		for i, mnt := range c.Mounts {
			mnt.Source = filepath.Clean(mnt.Source)
			mnt.Target = filepath.Clean(mnt.Target)
			c.Mounts[i] = mnt
		}

	}

	if themeSet {
		imports := config.GetStringSlicePreserveString(cfg, "theme")
		for _, imp := range imports {
			c.Imports = append(c.Imports, Import{
				Path: imp,
			})
		}

	}

	return
}

func removeDuplicatesKeepRight(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for i := len(in) - 1; i >= 0; i-- {
		v := in[i]
		if seen[v] {
			continue
		}
		out = append([]string{v}, out...)
		seen[v] = true
	}

	return out
}

func getStaticDirs(cfg config.Provider) []string {
	var staticDirs []string
	for i := -1; i <= 10; i++ {
		staticDirs = append(staticDirs, getStringOrStringSlice(cfg, "staticDir", i)...)
	}
	return staticDirs
}

func getStringOrStringSlice(cfg config.Provider, key string, id int) []string {

	if id >= 0 {
		key = fmt.Sprintf("%s%d", key, id)
	}

	return config.GetStringSlicePreserveString(cfg, key)

}
