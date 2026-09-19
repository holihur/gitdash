import { describe, expect, it } from "vitest";
import { packageSetupCommands, packageUseCommand } from "@/lib/package-command";
import { archiveKind, isPreviewableName } from "@/lib/package-files";

const base = { owner: "alice", name: "hello", version: "1.0.0" };
const origin = "http://host:8080";

describe("packageUseCommand", () => {
  it("为每个生态生成一键使用命令", () => {
    expect(packageUseCommand({ ...base, type: "npm" }, origin)).toBe(
      "npm install hello --registry http://host:8080/api/packages/npm/alice/",
    );
    expect(packageUseCommand({ ...base, type: "pypi" }, origin)).toBe(
      "pip install hello --index-url http://<user>:<PAT>@host:8080/api/packages/pypi/alice/simple",
    );
    expect(packageUseCommand({ ...base, type: "composer" }, origin)).toBe("composer require hello");
    expect(packageUseCommand({ ...base, type: "cargo" }, origin)).toBe(
      "cargo add hello --registry gitdash",
    );
    expect(packageUseCommand({ ...base, type: "go" }, origin)).toBe("go get hello@1.0.0");
    expect(packageUseCommand({ ...base, type: "rubygems" }, origin)).toBe(
      "gem install hello --source http://<user>:<PAT>@host:8080/api/packages/rubygems/alice",
    );
  });

  it("maven 把 group/artifact 路径还原为坐标", () => {
    expect(
      packageUseCommand({ type: "maven", owner: "alice", name: "com/example/hello", version: "1.2.3" }, origin),
    ).toBe("mvn dependency:get -DremoteRepositories=gitdash -Dartifact=com.example:hello:1.2.3");
  });
});

describe("packageSetupCommands", () => {
  it("需要一次性配置的生态给出前置命令", () => {
    expect(packageSetupCommands({ type: "composer", owner: "alice" }, origin)).toEqual([
      "composer config repositories.gitdash composer http://host:8080/api/packages/composer/alice",
    ]);
    expect(packageSetupCommands({ type: "npm", owner: "alice" }, origin)).toEqual([
      "npm config set //host:8080/:_authToken <PAT>",
    ]);
    expect(packageSetupCommands({ type: "go", owner: "alice" }, origin)).toEqual([]);
  });
});

describe("archiveKind", () => {
  it("按后缀识别归档类型", () => {
    expect(archiveKind("pkg-1.0.0.tgz")).toBe("tar.gz");
    expect(archiveKind("crate-0.1.0.crate")).toBe("tar.gz");
    expect(archiveKind("pkg-1.0.0-py3-none-any.whl")).toBe("zip");
    expect(archiveKind("lib-1.0.jar")).toBe("zip");
    expect(archiveKind("gem-1.0.0.gem")).toBe("tar");
    expect(archiveKind("LICENSE")).toBe("");
  });
});

describe("isPreviewableName", () => {
  it("文本文件可预览，二进制不可", () => {
    expect(isPreviewableName("package/package.json")).toBe(true);
    expect(isPreviewableName("Cargo.toml")).toBe(true);
    expect(isPreviewableName("go.mod")).toBe(true);
    expect(isPreviewableName("hello@v1.0.0/hello.go")).toBe(true);
    expect(isPreviewableName("logo.png")).toBe(false);
    expect(isPreviewableName("lib.class")).toBe(false);
  });
});
