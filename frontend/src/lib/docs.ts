import { useSyncExternalStore } from "react";

// 实例级只读信息（文档站地址）。单独成模块，避免被 @/lib/api 的测试 mock 影响。
//
// 启动时不再阻塞首次渲染去拉取实例信息，因此 docsURL 可能在首帧之后才写入。
// 这里做成极简的可订阅 store：`setDocsURL` 通知所有订阅者，`useDocsUrl` 让组件
// （登录页 / 页头 / 引导清单）在信息到达后自动补上文档入口。
let docsURL = "";
const listeners = new Set<() => void>();

/** 由 loadInstanceInfo 在启动时写入。 */
export function setDocsURL(v: string): void {
  if (v === docsURL) return;
  docsURL = v;
  for (const l of listeners) l();
}

/** 文档站地址（未配置返回空串）；非响应式读取，适合事件回调。 */
export function docsUrl(): string {
  return docsURL;
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

/** 响应式读取文档站地址：实例信息加载完成后会自动触发重渲染。 */
export function useDocsUrl(): string {
  return useSyncExternalStore(subscribe, docsUrl, docsUrl);
}
