// 实例级只读信息（文档站地址）。单独成模块，避免被 @/lib/api 的测试 mock 影响。
let docsURL = "";

/** 由 loadInstanceInfo 在启动时写入。 */
export function setDocsURL(v: string): void {
  docsURL = v;
}

/** 文档站地址（未配置返回空串）；登录页/页头据此显示文档入口。 */
export function docsUrl(): string {
  return docsURL;
}
