import { describe, expect, it } from "vitest";
import {
  COLLAB_ROLES,
  canAdmin,
  canMaintain,
  canRead,
  canTriage,
  canWrite,
  isRepoOwner,
  roleAtLeast,
} from "@/lib/repo-role";

describe("repo role ranking", () => {
  it("按等级排序", () => {
    expect(roleAtLeast("owner", "admin")).toBe(true);
    expect(roleAtLeast("admin", "maintain")).toBe(true);
    expect(roleAtLeast("maintain", "write")).toBe(true);
    expect(roleAtLeast("write", "triage")).toBe(true);
    expect(roleAtLeast("triage", "read")).toBe(true);
    expect(roleAtLeast("read", "triage")).toBe(false);
    expect(roleAtLeast(undefined, "read")).toBe(false);
    expect(roleAtLeast("bogus", "read")).toBe(false);
  });

  it("能力判断与等级一致", () => {
    expect(canRead("read")).toBe(true);
    expect(canTriage("read")).toBe(false);
    expect(canTriage("triage")).toBe(true);
    expect(canWrite("triage")).toBe(false);
    expect(canWrite("write")).toBe(true);
    expect(canMaintain("write")).toBe(false);
    expect(canMaintain("maintain")).toBe(true);
    expect(canAdmin("maintain")).toBe(false);
    expect(canAdmin("admin")).toBe(true);
    expect(isRepoOwner("admin")).toBe(false);
    expect(isRepoOwner("owner")).toBe(true);
  });

  it("可授予角色不含 owner", () => {
    expect(COLLAB_ROLES).toEqual(["read", "triage", "write", "maintain", "admin"]);
    expect(COLLAB_ROLES).not.toContain("owner");
  });
});
