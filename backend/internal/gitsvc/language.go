package gitsvc

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

// LanguageStat 单种语言的字节统计。
type LanguageStat struct {
	Language string
	Bytes    int64
}

// repoLanguages 计算 ref 上各语言的代码占比（按字节）；实际实现，供 cliBackend 调用。
//
// 只读取 `git ls-tree -r -l` 输出的 blob 大小，从不读取文件内容，因此对
// 大仓库也足够轻量；结果按字节数降序排列。
//
// 与 GitHub Linguist 的思路一致：按扩展名识别语言，并跳过 vendored /
// 生成的文件与文档、数据、配置等非代码文件。
//
// 返回空切片表示没有任何可识别的代码（空仓库同理）。
func repoLanguages(owner, name, ref string) ([]LanguageStat, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	p := repoPath(owner, name)
	out, err := gitOut(p, "ls-tree", "-r", "-l", "-z", ref)
	if err != nil {
		return nil, err
	}
	totals := map[string]int64{}
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		meta, file, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 4 || fields[1] != "blob" {
			continue
		}
		if fields[0] == "120000" { // 符号链接不计入统计
			continue
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil || size <= 0 {
			continue
		}
		if skipLanguagePath(file) {
			continue
		}
		lang := LanguageName(file)
		if lang == "" {
			continue
		}
		totals[lang] += size
	}
	stats := make([]LanguageStat, 0, len(totals))
	for lang, b := range totals {
		stats = append(stats, LanguageStat{Language: lang, Bytes: b})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Bytes != stats[j].Bytes {
			return stats[i].Bytes > stats[j].Bytes
		}
		return stats[i].Language < stats[j].Language
	})
	return stats, nil
}

// 被排除的目录段（小写比较）：依赖 / 生成物 / 构建产物等，避免污染代码占比。
var skipLanguageDirs = map[string]bool{
	"vendor":           true,
	"node_modules":     true,
	"bower_components": true,
	"third_party":      true,
	"thirdparty":       true,
	"externals":        true,
	".git":             true,
	".svn":             true,
	".hg":              true,
	"pods":             true,
	"carthage":         true,
	"__pycache__":      true,
	".venv":            true,
	"venv":             true,
	".tox":             true,
	".mypy_cache":      true,
	".pytest_cache":    true,
	"coverage":         true,
	".next":            true,
	".nuxt":            true,
	".cache":           true,
}

// 被排除的文件名（小写比较）：锁文件 / 压缩产物等。
var skipLanguageFiles = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"bun.lock":          true,
	"bun.lockb":         true,
	"go.sum":            true,
	"cargo.lock":        true,
	"composer.lock":     true,
	"gemfile.lock":      true,
	"poetry.lock":       true,
	"pipfile.lock":      true,
}

// skipLanguagePath 报告路径是否属于依赖 / 生成物 / 锁文件，应从语言统计中剔除。
func skipLanguagePath(file string) bool {
	base := strings.ToLower(path.Base(file))
	if skipLanguageFiles[base] {
		return true
	}
	if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.css") ||
		strings.HasSuffix(base, ".map") || strings.HasSuffix(base, ".generated.go") ||
		strings.HasSuffix(base, "_generated.go") || strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, ".pb.cc") || strings.HasSuffix(base, ".pb.h") ||
		strings.HasSuffix(base, "_pb2.py") || strings.HasSuffix(base, "_pb2_grpc.py") ||
		strings.HasSuffix(base, ".designer.cs") || strings.HasSuffix(base, ".g.cs") ||
		strings.HasSuffix(base, ".g.i.cs") || strings.HasSuffix(base, ".assemblyinfo.cs") {
		return true
	}
	for _, seg := range strings.Split(file, "/") {
		if skipLanguageDirs[strings.ToLower(seg)] {
			return true
		}
	}
	return false
}

// 特殊文件名 → 语言（不依赖扩展名）。
var specialLanguageFiles = map[string]string{
	"makefile":        "Makefile",
	"gnumakefile":     "Makefile",
	"dockerfile":      "Dockerfile",
	"cmakelists.txt":  "CMake",
	"rakefile":        "Ruby",
	"gemfile":         "Ruby",
	"vagrantfile":     "Ruby",
	"justfile":        "Just",
	"meson.build":     "Meson",
	"build":           "Starlark",
	"workspace":       "Starlark",
	"build.bazel":     "Starlark",
	"workspace.bazel": "Starlark",
}

// 扩展名（小写、不含点）→ 语言。仅收录“代码/标记”类语言；文档、数据、
// 配置类（md/json/yaml/xml/toml/ini/csv/txt...）一律不计数。
var extLanguages = map[string]string{
	// 系统 / 底层
	"go":  "Go",
	"c":   "C",
	"h":   "C",
	"cc":  "C++",
	"cpp": "C++",
	"cxx": "C++",
	"c++": "C++",
	"hpp": "C++",
	"hh":  "C++",
	"hxx": "C++",
	"h++": "C++",
	"ipp": "C++",
	"m":   "Objective-C",
	"mm":  "Objective-C++",
	"rs":  "Rust",
	"zig": "Zig",
	"nim": "Nim",
	"v":   "V",
	"d":   "D",
	"cr":  "Crystal",
	"asm": "Assembly",
	"s":   "Assembly",

	// JVM / .NET
	"cs":     "C#",
	"java":   "Java",
	"kt":     "Kotlin",
	"kts":    "Kotlin",
	"scala":  "Scala",
	"sc":     "Scala",
	"groovy": "Groovy",
	"gvy":    "Groovy",
	"gradle": "Gradle",
	"fs":     "F#",
	"fsi":    "F#",
	"fsx":    "F#",
	"vb":     "Visual Basic .NET",
	"vbs":    "VBScript",

	// Web / 脚本
	"js":         "JavaScript",
	"mjs":        "JavaScript",
	"cjs":        "JavaScript",
	"jsx":        "JavaScript",
	"ts":         "TypeScript",
	"tsx":        "TypeScript",
	"vue":        "Vue",
	"svelte":     "Svelte",
	"astro":      "Astro",
	"html":       "HTML",
	"htm":        "HTML",
	"xhtml":      "HTML",
	"css":        "CSS",
	"scss":       "SCSS",
	"sass":       "Sass",
	"less":       "Less",
	"hbs":        "Handlebars",
	"handlebars": "Handlebars",
	"ejs":        "EJS",
	"erb":        "ERB",
	"jinja":      "Jinja",
	"j2":         "Jinja",
	"twig":       "Twig",
	"php":        "PHP",
	"phtml":      "PHP",
	"php3":       "PHP",
	"php4":       "PHP",
	"php5":       "PHP",

	// 脚本语言
	"py":      "Python",
	"pyw":     "Python",
	"pyi":     "Python",
	"rb":      "Ruby",
	"rake":    "Ruby",
	"gemspec": "Ruby",
	"pl":      "Perl",
	"pm":      "Perl",
	"lua":     "Lua",
	"r":       "R",
	"jl":      "Julia",
	"sh":      "Shell",
	"bash":    "Shell",
	"zsh":     "Shell",
	"ksh":     "Shell",
	"fish":    "Shell",
	"ps1":     "PowerShell",
	"psm1":    "PowerShell",
	"psd1":    "PowerShell",
	"bat":     "Batchfile",
	"cmd":     "Batchfile",
	"awk":     "Awk",
	"tcl":     "Tcl",

	// 函数式 / 其他
	"hs":   "Haskell",
	"lhs":  "Haskell",
	"ex":   "Elixir",
	"exs":  "Elixir",
	"erl":  "Erlang",
	"hrl":  "Erlang",
	"clj":  "Clojure",
	"cljs": "Clojure",
	"cljc": "Clojure",
	"edn":  "Clojure",
	"ml":   "OCaml",
	"mli":  "OCaml",
	"scm":  "Scheme",
	"ss":   "Scheme",
	"rkt":  "Racket",
	"lisp": "Common Lisp",
	"lsp":  "Common Lisp",
	"cl":   "Common Lisp",
	"el":   "Emacs Lisp",
	"vim":  "Vim Script",

	// 移动 / 游戏
	"swift": "Swift",
	"dart":  "Dart",
	"gd":    "GDScript",
	"qml":   "QML",
	"sol":   "Solidity",

	// 数据 / 查询 / 接口
	"sql":     "SQL",
	"proto":   "Protocol Buffer",
	"graphql": "GraphQL",
	"gql":     "GraphQL",
	"thrift":  "Thrift",

	// 构建 / 基础设施
	"cmake":  "CMake",
	"mk":     "Makefile",
	"tf":     "HCL",
	"tfvars": "HCL",
	"hcl":    "HCL",
	"nix":    "Nix",
	"cue":    "CUE",
	"wat":    "WebAssembly",
	"tex":    "TeX",
	"latex":  "TeX",
	"sty":    "TeX",
	"cls":    "TeX",

	// 传统语言
	"f":   "Fortran",
	"f77": "Fortran",
	"f90": "Fortran",
	"f95": "Fortran",
	"f03": "Fortran",
	"cob": "COBOL",
	"cbl": "COBOL",
	"pas": "Pascal",
	"pp":  "Pascal",
	"adb": "Ada",
	"ads": "Ada",
	"pro": "Prolog",
}

// KnownLanguages 返回检测器支持的全部语言展示名（去重、排序），
// 供管理端配置语言配色时列出可选项。
func KnownLanguages() []string {
	set := make(map[string]bool, len(extLanguages)+len(specialLanguageFiles))
	for _, name := range extLanguages {
		set[name] = true
	}
	for _, name := range specialLanguageFiles {
		set[name] = true
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// defaultLanguageColors 常见语言的默认配色（与 GitHub 常用色一致）。
// 这是全站默认配色的唯一数据源，管理端只在此之上做覆盖。
var defaultLanguageColors = map[string]string{
	"Go":                "#00ADD8",
	"C":                 "#555555",
	"C++":               "#f34b7d",
	"C#":                "#178600",
	"Objective-C":       "#438eff",
	"Objective-C++":     "#6866fb",
	"Java":              "#b07219",
	"Kotlin":            "#A97BFF",
	"Scala":             "#c22d40",
	"Groovy":            "#4298b8",
	"Gradle":            "#02303a",
	"JavaScript":        "#f1e05a",
	"TypeScript":        "#3178c6",
	"Python":            "#3572A5",
	"Ruby":              "#701516",
	"PHP":               "#4F5D95",
	"Rust":              "#dea584",
	"Swift":             "#F05138",
	"Dart":              "#00B4AB",
	"Lua":               "#000080",
	"Perl":              "#0298c3",
	"R":                 "#198CE7",
	"Julia":             "#a270ba",
	"Haskell":           "#5e5086",
	"Elixir":            "#6e4a7e",
	"Erlang":            "#B83998",
	"Clojure":           "#db5855",
	"F#":                "#b845fc",
	"Visual Basic .NET": "#945db7",
	"HTML":              "#e34c26",
	"CSS":               "#563d7c",
	"SCSS":              "#c6538c",
	"Sass":              "#a53b70",
	"Less":              "#1d365d",
	"Vue":               "#41b883",
	"Svelte":            "#ff3e00",
	"Astro":             "#ff5a03",
	"Shell":             "#89e051",
	"PowerShell":        "#012456",
	"Batchfile":         "#C1F12E",
	"Makefile":          "#427819",
	"Dockerfile":        "#384d54",
	"CMake":             "#DA3434",
	"SQL":               "#e38c00",
	"Protocol Buffer":   "#6b5f9e",
	"GraphQL":           "#e10098",
	"HCL":               "#844FBA",
	"Nix":               "#7e7eff",
	"Zig":               "#ec915c",
	"Nim":               "#ffc200",
	"Solidity":          "#AA6746",
	"Assembly":          "#6E4C13",
	"TeX":               "#3D6117",
}

// languageColorPalette 未固定配色的语言按名称哈希稳定落到该调色板。
var languageColorPalette = []string{
	"#0e8a16",
	"#1f77b4",
	"#d62728",
	"#9467bd",
	"#8c564b",
	"#e377c2",
	"#7f7f7f",
	"#bcbd22",
	"#17becf",
	"#ff7f0e",
	"#2ca02c",
	"#6a3d9a",
}

// DefaultLanguageColor 返回语言的默认颜色：优先固定配色表，否则按名称哈希稳定选取。
func DefaultLanguageColor(lang string) string {
	if c, ok := defaultLanguageColors[lang]; ok {
		return c
	}
	var h uint32
	for _, r := range lang {
		h = h*31 + uint32(r)
	}
	return languageColorPalette[h%uint32(len(languageColorPalette))]
}

// DefaultLanguageColors 返回全部已知语言的默认配色快照（含哈希回退色）。
func DefaultLanguageColors() map[string]string {
	langs := KnownLanguages()
	out := make(map[string]string, len(langs))
	for _, l := range langs {
		out[l] = DefaultLanguageColor(l)
	}
	return out
}

// LanguageName 返回文件对应的展示用语言名；未知或非代码文件返回空串。
func LanguageName(file string) string {
	base := strings.ToLower(path.Base(file))
	if lang, ok := specialLanguageFiles[base]; ok {
		return lang
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(base), "."))
	if ext == "" {
		return ""
	}
	return extLanguages[ext]
}
