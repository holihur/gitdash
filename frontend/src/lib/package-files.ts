/** 归档类型识别（与后端 archiveKind 保持一致）。 */
export function archiveKind(filename: string): "zip" | "tar" | "tar.gz" | "" {
  const lower = filename.toLowerCase();
  if (lower.endsWith(".tar.gz") || lower.endsWith(".tgz") || lower.endsWith(".crate")) return "tar.gz";
  if (lower.endsWith(".zip") || lower.endsWith(".jar") || lower.endsWith(".whl")) return "zip";
  if (lower.endsWith(".tar") || lower.endsWith(".gem")) return "tar";
  return "";
}

const TEXT_EXT = [
  ".txt", ".md", ".markdown", ".json", ".xml", ".pom", ".toml", ".yml", ".yaml", ".cfg", ".ini",
  ".conf", ".log", ".env", ".mod", ".sum", ".go", ".py", ".rb", ".php", ".java", ".rs", ".ts",
  ".tsx", ".jsx", ".js", ".mjs", ".cjs", ".c", ".h", ".cpp", ".hpp", ".sh", ".bash", ".zsh",
  ".sql", ".gemspec", ".lock", ".css", ".html", ".htm", ".csv",
];

/** 是否可按文本预览（用于决定展示预览还是仅提供下载）。 */
export function isPreviewableName(name: string): boolean {
  const base = (name.split("/").pop() ?? name).toLowerCase();
  if (["go.mod", "go.sum", "cargo.toml", "cargo.lock", "gemfile", "rakefile", "makefile", "readme", "license", "licence", "notice", "changelog"].includes(base)) {
    return true;
  }
  return TEXT_EXT.some((ext) => base.endsWith(ext));
}
