import "@testing-library/jest-dom/vitest";
import { beforeEach } from "vitest";
import { clearApiCache } from "@/lib/api/core";

// 客户端 GET 缓存是模块级的，测试之间需要清空，避免上一个用例的响应被复用。
beforeEach(() => {
  clearApiCache();
});

// jsdom 未实现 matchMedia（theme system 模式需要）
if (!window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList;
}
