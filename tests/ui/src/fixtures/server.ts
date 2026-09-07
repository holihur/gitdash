import { randomUUID } from "node:crypto";
import { appendFileSync, mkdtempSync, rmSync } from "node:fs";
import { spawn, type ChildProcess } from "node:child_process";
import { createServer, type AddressInfo } from "node:net";
import { tmpdir } from "node:os";
import path from "node:path";

/**
 * 实例夹具（对齐 tests/conftest.py 的约定）：
 * - GITDASH_BIN    指向已构建的 gitdash 可执行文件：在独立临时数据目录 + 随机端口上
 *                  启动全新实例，进程结束时自动终止并清理。
 * - GITDASH_UI_URL 指向已运行实例（本地 dev / CI 容器）：只做 UI 驱动。
 * 两者都未设置时所有用例 skip（spec 顶层 test.skip 处理）。
 */

export interface GitdashInstance {
  /** http://127.0.0.1:PORT（无结尾斜杠） */
  baseURL: string;
  /** 由本夹具自启时为进程句柄；外部实例模式为 null */
  proc: ChildProcess | null;
  /** 自启实例的临时数据目录（外部实例模式为 null） */
  dataDir: string | null;
}

/** 每个进程内仅启动一个实例（worker 级 fixture 复用） */
let cached: Promise<GitdashInstance> | null = null;

export function spawnOrUseExisting(): Promise<GitdashInstance> {
  cached ??= startInstanceFromEnv();
  return cached;
}

async function startInstanceFromEnv(): Promise<GitdashInstance> {
  const bin = (process.env.GITDASH_BIN ?? "").trim();
  const url = (process.env.GITDASH_UI_URL ?? "").trim();
  if (url) return { baseURL: url.replace(/\/+$/, ""), proc: null, dataDir: null };
  if (bin) return spawnServer(bin);
  throw new Error(
    "UI tests need a server instance: set GITDASH_BIN (prebuilt binary) or GITDASH_UI_URL (already running instance)",
  );
}

async function spawnServer(binary: string): Promise<GitdashInstance> {
  const httpPort = await freePort();
  const sshPort = await freePort();
  const dataDir = mkdtempSync(path.join(tmpdir(), "gitdash-ui-"));
  const logPath = path.join(dataDir, "server.log");
  const proc = spawn(binary, ["serve"], {
    env: {
      ...process.env,
      GITDASH_DATA: path.join(dataDir, "data"),
      GITDASH_DISABLE_RATE_LIMIT: "1",
      GITDASH_HTTP_ADDR: `127.0.0.1:${httpPort}`,
      GITDASH_SSH_ADDR: `127.0.0.1:${sshPort}`,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  proc.stdout!.on("data", (c) => appendLog(logPath, c));
  proc.stderr!.on("data", (c) => appendLog(logPath, c));
  // 就绪探测（唯一允许的直连请求：与 e2e.sh / conftest.py 一致，仅用于等待启动）
  const baseURL = `http://127.0.0.1:${httpPort}`;
  const deadline = Date.now() + 30_000;
  for (;;) {
    if (proc.exitCode !== null) {
      throw new Error(`server exited rc=${proc.exitCode}; see ${logPath}`);
    }
    try {
      const res = await fetch(`${baseURL}/api/health`);
      if (res.ok) break;
    } catch {
      /* not ready yet */
    }
    if (Date.now() > deadline) {
      proc.kill();
      throw new Error(`server not ready in time; see ${logPath}`);
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  return { baseURL, proc, dataDir };
}

function appendLog(logPath: string, chunk: Buffer) {
  try {
    appendFileSync(logPath, chunk);
  } catch {
    /* ignore */
  }
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = createServer();
    srv.listen(0, "127.0.0.1", () => {
      const port = (srv.address() as AddressInfo).port;
      srv.close(() => resolve(port));
    });
    srv.on("error", reject);
  });
}

/** 实例生命周期收尾：先 SIGTERM（让 GOCOVERDIR 覆盖率落盘），超时再 SIGKILL */
export function cleanupInstance(inst: GitdashInstance): Promise<void> {
  if (inst.proc && inst.proc.exitCode === null) {
    return new Promise((resolve) => {
      const p = inst.proc!;
      let killed = false;
      const done = () => {
        if (inst.dataDir) rmDataDir(inst.dataDir);
        resolve();
      };
      p.once("exit", done);
      p.kill(); // SIGTERM → 服务端优雅停机并 flush 覆盖率
      setTimeout(() => {
        if (p.exitCode === null && !killed) {
          killed = true;
          p.kill("SIGKILL");
          done();
        }
      }, 10_000);
    });
  }
  if (inst.dataDir) rmDataDir(inst.dataDir);
  return Promise.resolve();
}

function rmDataDir(dir: string) {
  try {
    rmSync(dir, { recursive: true, force: true });
  } catch {
    /* ignore */
  }
}

/** 用户名/仓库名随机生成（对齐 API 测试的隔离原则） */
export function randUser(): string {
  return `ui-${randomUUID().replace(/-/g, "").slice(0, 10)}`;
}

export function randRepo(): string {
  return `repo-${randomUUID().replace(/-/g, "").slice(0, 8)}`;
}
