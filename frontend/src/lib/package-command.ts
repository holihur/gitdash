import type { PackageEntry } from "@/lib/api/types";

/** 包在浏览器里的 origin（如 http://host:8080）。 */
export function currentOrigin(): string {
  return typeof window !== "undefined" ? window.location.origin : "http://localhost:8080";
}

function hostOf(origin: string): string {
  try {
    return new URL(origin).host;
  } catch {
    return origin.replace(/^https?:\/\//, "");
  }
}

/**
 * 每个生态对应的「一键使用」命令（复制到终端即可安装/依赖该包）。
 * 私有仓库需要 Basic 认证，故 pypi / rubygems 用 <user>:<PAT> 占位符；
 * npm / composer / cargo 需要先做一次性配置，见 packageSetupCommands。
 */
export function packageUseCommand(
  p: Pick<PackageEntry, "type" | "owner" | "name" | "version">,
  origin: string = currentOrigin(),
): string {
  const host = hostOf(origin);
  switch (p.type) {
    case "npm":
      return `npm install ${p.name} --registry ${origin}/api/packages/npm/${p.owner}/`;
    case "pypi":
      return `pip install ${p.name} --index-url http://<user>:<PAT>@${host}/api/packages/pypi/${p.owner}/simple`;
    case "composer":
      return `composer require ${p.name}`;
    case "cargo":
      return `cargo add ${p.name} --registry gitdash`;
    case "go":
      return `go get ${p.name}@${p.version}`;
    case "rubygems":
      return `gem install ${p.name} --source http://<user>:<PAT>@${host}/api/packages/rubygems/${p.owner}`;
    case "maven": {
      const parts = p.name.split("/");
      const artifact = parts[parts.length - 1];
      const group = parts.slice(0, -1).join(".");
      return `mvn dependency:get -DremoteRepositories=gitdash -Dartifact=${group}:${artifact}:${p.version}`;
    }
    default:
      return `${p.name}@${p.version}`;
  }
}

/**
 * 需要一次性配置的生态返回前置命令（可为空数组）。
 * 例如 npm 需要在 .npmrc 配置 registry / authToken。
 */
export function packageSetupCommands(
  p: Pick<PackageEntry, "type" | "owner">,
  origin: string = currentOrigin(),
): string[] {
  const host = hostOf(origin);
  switch (p.type) {
    case "composer":
      return [
        `composer config repositories.gitdash composer ${origin}/api/packages/composer/${p.owner}`,
      ];
    case "cargo":
      return [
        `# .cargo/config.toml`,
        `[registries.gitdash]`,
        `index = "sparse+${origin}/api/packages/cargo/${p.owner}/index/"`,
      ];
    case "npm":
      return [`npm config set //${host}/:_authToken <PAT>`];
    default:
      return [];
  }
}
