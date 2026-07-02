import { execSync } from "child_process";
import { existsSync, rmSync } from "fs";

const REPO_URL = "https://github.com/ProtonMail/WebClients.git";

function isGitRepo(dest: string): boolean {
  if (!existsSync(`${dest}/.git`)) return false;
  try {
    execSync("git rev-parse --is-inside-work-tree", {
      cwd: dest,
      stdio: "pipe",
    });
    return true;
  } catch {
    return false;
  }
}

function clone(dest: string): void {
  execSync(`git clone --depth 1 --branch main ${REPO_URL} ${dest}`, {
    stdio: "pipe",
  });
}

export function ensureRepo(dest: string): void {
  if (!isGitRepo(dest)) {
    rmSync(dest, { recursive: true, force: true });
    clone(dest);
    return;
  }
  try {
    execSync("git pull", { cwd: dest, stdio: "pipe" });
  } catch {
    rmSync(dest, { recursive: true, force: true });
    clone(dest);
  }
}
