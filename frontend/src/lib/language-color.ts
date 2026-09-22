/**
 * 语言配色的**内置兜底**：后端 `GET /api/languages` 返回的 `defaults`
 * 才是权威默认色；这里仅在后端不可用或响应到达前使用，避免配色闪烁。
 * 与 GitHub 常用色保持一致；未知语言按名称哈希落到调色板，保证稳定。
 */
const KNOWN: Record<string, string> = {
  Go: "#00ADD8",
  C: "#555555",
  "C++": "#f34b7d",
  "C#": "#178600",
  "Objective-C": "#438eff",
  "Objective-C++": "#6866fb",
  Java: "#b07219",
  Kotlin: "#A97BFF",
  Scala: "#c22d40",
  Groovy: "#4298b8",
  Gradle: "#02303a",
  JavaScript: "#f1e05a",
  TypeScript: "#3178c6",
  Python: "#3572A5",
  Ruby: "#701516",
  PHP: "#4F5D95",
  Rust: "#dea584",
  Swift: "#F05138",
  Dart: "#00B4AB",
  Lua: "#000080",
  Perl: "#0298c3",
  R: "#198CE7",
  Julia: "#a270ba",
  Haskell: "#5e5086",
  Elixir: "#6e4a7e",
  Erlang: "#B83998",
  Clojure: "#db5855",
  "F#": "#b845fc",
  "Visual Basic .NET": "#945db7",
  HTML: "#e34c26",
  CSS: "#563d7c",
  SCSS: "#c6538c",
  Sass: "#a53b70",
  Less: "#1d365d",
  Vue: "#41b883",
  Svelte: "#ff3e00",
  Astro: "#ff5a03",
  Shell: "#89e051",
  PowerShell: "#012456",
  Batchfile: "#C1F12E",
  Makefile: "#427819",
  Dockerfile: "#384d54",
  CMake: "#DA3434",
  SQL: "#e38c00",
  "Protocol Buffer": "#6b5f9e",
  GraphQL: "#e10098",
  HCL: "#844FBA",
  Nix: "#7e7eff",
  Zig: "#ec915c",
  Nim: "#ffc200",
  Solidity: "#AA6746",
  Assembly: "#6E4C13",
  TeX: "#3D6117",
};

const PALETTE = [
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
];

/** 返回语言对应的十六进制颜色。 */
export function languageColor(lang: string): string {
  const known = KNOWN[lang];
  if (known) return known;
  let h = 0;
  for (let i = 0; i < lang.length; i++) h = (h * 31 + lang.charCodeAt(i)) >>> 0;
  return PALETTE[h % PALETTE.length];
}
