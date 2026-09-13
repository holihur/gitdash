import { describe, expect, it } from "vitest";
import { extractOutline } from "@/lib/outline";

describe("extractOutline", () => {
  it("Markdown 抽取标题并生成锚点 id", () => {
    const items = extractOutline(
      "README.md",
      "# Top\n\n## Second\n\n### Third\n\n```\n# not a heading\n```\n",
    );
    expect(items.map((i) => i.text)).toEqual(["Top", "Second", "Third"]);
    expect(items[0].kind).toBe("heading");
    expect(items[0].id).toBe("md-heading-0");
    expect(items[1].level).toBe(1);
  });

  it("Go 抽取 func / type / var", () => {
    const items = extractOutline(
      "main.go",
      [
        "package main",
        "type Server struct {",
        "}",
        "func (s *Server) Run() {",
        "}",
        "func NewServer() *Server {",
        "}",
        "const DefaultPort = 8080",
      ].join("\n"),
    );
    const kinds = items.map((i) => `${i.kind}:${i.text}`);
    expect(kinds).toEqual([
      "type:Server",
      "function:Run",
      "function:NewServer",
      "variable:DefaultPort",
    ]);
    // Go 方法/函数都在顶层（无缩进）
    expect(items[1].level).toBe(0);
    // 行号 1-based
    expect(items[1].line).toBe(4);
  });

  it("TypeScript 抽取 class / function / 箭头函数", () => {
    const items = extractOutline(
      "app.ts",
      "export class Store {\n}\nexport function load() {\n}\nexport const api = () => {};\n",
    );
    expect(items.map((i) => `${i.kind}:${i.text}`)).toEqual([
      "type:Store",
      "function:load",
      "variable:api",
    ]);
  });

  it("Python 抽取 class / def 并按缩进分层", () => {
    const items = extractOutline(
      "app.py",
      "class App:\n    def run(self):\n        pass\ndef main():\n    pass\n",
    );
    expect(items.map((i) => `${i.kind}:${i.text}`)).toEqual([
      "type:App",
      "function:run",
      "function:main",
    ]);
    expect(items[1].level).toBe(1);
    expect(items[2].level).toBe(0);
  });

  it("YAML 抽取顶层 key", () => {
    const items = extractOutline(
      ".gitdash.yml",
      "image: alpine:3.19\nsteps:\n  - name: build\non:\n  push:\n",
    );
    expect(items.map((i) => i.text)).toEqual(["image", "steps", "on"]);
  });

  it("不支持的扩展名返回空", () => {
    expect(extractOutline("data.bin", "whatever")).toEqual([]);
  });
});
