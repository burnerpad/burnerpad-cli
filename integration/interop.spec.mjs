import { test, expect } from "@playwright/test";
import { spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";

const cli = process.env.BURNERPAD_BIN;
const server = process.env.BROWSER_TEST_BASE_URL;
const phrase = "aardvark carrot embroidery hardhat lyrics porcupine suave";

if (!cli || !server) throw new Error("BURNERPAD_BIN and BROWSER_TEST_BASE_URL are required");

function protectedFile(name, value) {
  const directory = mkdtempSync(join(tmpdir(), "burnerpad-interop-"));
  const path = join(directory, name);
  writeFileSync(path, value + "\n", { mode: 0o600 });
  chmodSync(path, 0o600);
  return path;
}

function runRaw(args, input) {
  const options = {
    encoding: "utf8",
    env: { ...process.env, BURNERPAD_SERVER: "" },
    timeout: 30_000,
    stdio: input === undefined ? ["ignore", "pipe", "pipe"] : ["pipe", "pipe", "pipe"]
  };
  if (input !== undefined) options.input = input;
  const result = spawnSync(cli, args, options);
  return result;
}

function run(args, input) {
  const result = runRaw(args, input);
  if (result.status !== 0) {
    throw new Error(`CLI failed (${result.status}): ${result.stderr}`);
  }
  return { ...result, json: JSON.parse(result.stdout) };
}

function runError(args, input, exit, code) {
  const result = runRaw(args, input);
  expect(result.status).toBe(exit);
  expect(JSON.parse(result.stdout).code).toBe(code);
  return result;
}

async function pastePhrase(page, words) {
  await page.locator("#bp-psk-input").evaluate((input, pasted) => {
    input.focus();
    const data = new DataTransfer();
    data.setData("text/plain", pasted);
    const event = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(event, "clipboardData", { value: data });
    input.dispatchEvent(event);
  }, words);
}

test("browser create is revealed by Go CLI", async ({ page }) => {
  const plaintext = "browser to Go CLI 🧪";
  await page.goto("/");
  await page.fill("#bp-input", plaintext);
  const words = (await page.locator("#bp-pass-chips .chip").allTextContents()).map((word) => word.replace("×", "").trim());
  expect(words).toHaveLength(7);
  await page.getByRole("button", { name: "Encrypt & create link" }).click();
  await expect(page.locator("#bp-result")).toBeVisible();
  const link = await page.locator("#bp-link").getAttribute("data-full-url");
  const result = run(["reveal", "--json", "--passphrase-file", protectedFile("phrase", words.join(" ")), link]);
  expect(result.json).toEqual({ status: "revealed", server, plaintext });
});

test("Go CLI create is revealed by browser", async ({ page }) => {
  const plaintext = "Go CLI to browser 🧪";
  const created = run(["create", "--server", server, "--json"], plaintext).json;
  expect(created.status).toBe("created");
  expect(created.server).toBe(server);
  await page.goto(created.link);
  await expect(page.locator("#bp-psk-input")).toBeFocused();
  await pastePhrase(page, created.phrase);
  await page.locator("#bp-psk-reveal").click();
  await expect(page.locator("#bp-revealed")).toBeVisible();
  await expect(page.locator("#bp-secret")).toHaveText(plaintext);
});

test("one Go CLI creates and another reveals", async () => {
  const plaintext = "Go CLI to Go CLI";
  const created = run(["create", "--server", server, "--json"], plaintext).json;
  const phraseFile = protectedFile("phrase", created.phrase);
  const revealed = run(["reveal", "--json", "--passphrase-file", phraseFile, created.link]).json;
  expect(revealed).toEqual({ status: "revealed", server, plaintext });
  runError(["reveal", "--json", "--passphrase-file", phraseFile, created.link], undefined, 4, "secret_unavailable");
});

test("create receipts, full URLs, and bare normalized IDs all burn", async () => {
  const phraseFile = protectedFile("phrase", phrase);

  const receipt = run(["create", "--server", server, "--json"], "receipt burn").json;
  expect(run(["burn", "--json"], JSON.stringify(receipt)).json).toEqual({ status: "burned", server });
  runError(["reveal", "--json", "--passphrase-file", phraseFile, receipt.link], undefined, 4, "secret_unavailable");

  const fullURL = run(["create", "--server", server, "--json"], "URL burn").json;
  expect(run(["burn", "--json", "--token-file", protectedFile("token", fullURL.mgmt_token), fullURL.link]).json)
    .toEqual({ status: "burned", server });

  const bareID = run(["create", "--server", server, "--json"], "ID burn").json;
  const id = new URL(bareID.link).pathname.slice(3).toLowerCase();
  const wrong = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA";
  runError(["burn", "--server", server, "--json", "--token-file", protectedFile("wrong-token", wrong), id], undefined, 4, "secret_unavailable");
  expect(run(["burn", "--server", server, "--json", "--token-file", protectedFile("token", bareID.mgmt_token), id]).json)
    .toEqual({ status: "burned", server });
});

test("server effective TTL is returned for default and clamped requests", async () => {
  for (const [requested, effective] of [[null, 86400], ["1", 60], ["999999999", 86400]]) {
    const args = ["create", "--server", server, "--json"];
    if (requested !== null) args.push("--ttl", requested);
    const created = run(args, `ttl ${requested ?? "default"}`).json;
    expect(created.ttl).toBe(effective);
    run(["burn", "--json"], JSON.stringify(created));
  }
});
