import "@testing-library/jest-dom/vitest";
import { beforeEach, vi } from "vitest";
import { clearApiCache } from "@/lib/api/core";

// 单测环境（jsdom）无法真实挂载 CodeMirror：用受控 textarea 替身，
// 保持与真实编辑器一致的 value/onChange 与命令句柄接口。
vi.mock("@/components/markdown-code-editor", async () => {
  const React = await import("react");
  const MockEditor = React.forwardRef(function MockEditor(props: any, ref: any) {
    const propsRef = React.useRef(props);
    propsRef.current = props;
    React.useImperativeHandle(
      ref,
      () => ({
        focus: () => undefined,
        wrapSelection: (before: string, after = before, placeholder = "") =>
          propsRef.current.onChange(`${propsRef.current.value ?? ""}${before}${placeholder}${after}`),
        toggleLinePrefix: (prefix: string) =>
          propsRef.current.onChange(`${prefix}${propsRef.current.value ?? ""}`),
        insertBlock: (text: string) =>
          propsRef.current.onChange(`${propsRef.current.value ?? ""}\n${text}`),
        insertLink: () => propsRef.current.onChange(`${propsRef.current.value ?? ""}[text](url)`),
        insertText: (text: string) => propsRef.current.onChange(`${propsRef.current.value ?? ""}${text}`),
      }),
      [],
    );
    return React.createElement("textarea", {
      value: props.value,
      placeholder: props.placeholder,
      autoFocus: props.autoFocus,
      onChange: (e: { target: { value: string } }) => props.onChange(e.target.value),
    });
  });
  return { default: MockEditor };
});

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
