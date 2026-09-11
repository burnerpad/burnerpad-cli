import { test, expect } from "@playwright/test";
import { spawnSync } from "node:child_process";
import { chmodSync, mkdtempSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";

const cli = process.env.BURNERPAD_BIN;
const server = process.env.BROWSER_TEST_BASE_URL;
const expiryServer = process.env.BROWSER_TEST_EXPIRY_BASE_URL;
const phrase = "aardvark carrot embroidery hardhat lyrics porcupine suave";
// A random 26-character ID misses one requested digit with probability (31/32)^26.
// This cap puts each target's miss chance below 1.2e-23 and bounds three CI attempts at 576 creates.
const aliasAttemptLimit = 64;
const canonicalIDPattern = /^[0123456789ABCDEFGHJKMNPQRSTVWXYZ]{26}$/;
const managementTokenPattern = /^[A-Za-z0-9_-]{43}$/;

if (!cli || !server || !expiryServer) {
  throw new Error("BURNERPAD_BIN, BROWSER_TEST_BASE_URL, and BROWSER_TEST_EXPIRY_BASE_URL are required");
}

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

async function createAliasRow(request) {
  let response;
  try {
    response = await request.post("/api/secrets", { data: { blob: "AA", ttl: 60 } });
  } catch {
    throw new Error("alias fixture create request failed");
  }
  if (response.status() !== 200) {
    await response.dispose();
    throw new Error("alias fixture create failed");
  }

  let row;
  try {
    row = await response.json();
  } catch {
    throw new Error("alias fixture create returned invalid JSON");
  } finally {
    await response.dispose();
  }
  if (!row || !canonicalIDPattern.test(row.id) || !managementTokenPattern.test(row.mgmt_token) || row.ttl !== 60) {
    throw new Error("alias fixture create returned an invalid row");
  }
  return { id: row.id, managementToken: row.mgmt_token };
}

async function burnAliasRow(request, row, allowMissing = false) {
  let status;
  try {
    const response = await request.post(`/api/secrets/${row.id}/burn`, {
      data: { mgmt_token: row.managementToken }
    });
    status = response.status();
    await response.dispose();
  } catch {
    throw new Error("alias fixture cleanup request failed");
  }
  if (status !== 200 && !(allowMissing && status === 404)) {
    throw new Error("alias fixture cleanup failed");
  }
  return status;
}

async function findAliasRow(request, canonicalCharacter, alias) {
  for (let attempt = 0; attempt < aliasAttemptLimit; attempt++) {
    const row = await createAliasRow(request);
    if (row.id.includes(canonicalCharacter)) return row;
    await burnAliasRow(request, row);
  }
  throw new Error(`could not mint a row for the ${alias} alias within ${aliasAttemptLimit} attempts`);
}

function assertSafeAliasBurn(alias, row, aliasedID, result) {
  if (result.status !== 0) throw new Error(`CLI burn failed for the ${alias} alias`);
  for (const stream of [result.stdout, result.stderr]) {
    if (typeof stream === "string" && [row.id, row.managementToken, aliasedID].some((value) => stream.includes(value))) {
      throw new Error(`CLI output exposed a capability for the ${alias} alias`);
    }
  }

  const expectedStdout = JSON.stringify({ status: "burned", server }) + "\n";
  const expectedStderr = `burnerpad: server: ${server}\n`;
  if (result.stdout !== expectedStdout || result.stderr !== expectedStderr) {
    throw new Error(`CLI burn returned an unexpected result for the ${alias} alias`);
  }
}

async function expiryStats(request) {
  let response;
  try {
    response = await request.get(`${expiryServer}/api/stats`);
  } catch {
    throw new Error("expiry fixture stats request failed");
  }
  if (response.status() !== 200) {
    await response.dispose();
    throw new Error("expiry fixture stats request was rejected");
  }

  let stats;
  try {
    stats = await response.json();
  } catch {
    throw new Error("expiry fixture stats returned invalid JSON");
  } finally {
    await response.dispose();
  }
  for (const field of ["resident", "created", "claimed", "expired"]) {
    if (!Number.isSafeInteger(stats?.[field]) || stats[field] < 0) {
      throw new Error("expiry fixture stats omitted a required counter");
    }
  }
  return stats;
}

async function expiryPageStatus(request, link) {
  try {
    const response = await request.get(link);
    const status = response.status();
    await response.dispose();
    return status;
  } catch {
    throw new Error("expiry fixture liveness request failed");
  }
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

test("Go CLI burns real Lite rows through every Crockford alias", async ({ request }) => {
  for (const { canonicalCharacter, alias } of [
    { canonicalCharacter: "1", alias: "I" },
    { canonicalCharacter: "1", alias: "L" },
    { canonicalCharacter: "0", alias: "O" }
  ]) {
    const row = await findAliasRow(request, canonicalCharacter, alias);
    let aliasBurned = false;
    try {
      const aliasedID = row.id.replace(canonicalCharacter, alias);
      const result = runRaw([
        "burn", "--server", server, "--json",
        "--token-file", protectedFile("token", row.managementToken),
        aliasedID
      ]);
      assertSafeAliasBurn(alias, row, aliasedID, result);
      aliasBurned = true;
    } finally {
      const cleanupStatus = await burnAliasRow(request, row, true);
      if (aliasBurned && cleanupStatus !== 404) {
        throw new Error(`CLI burn did not revoke the selected row for the ${alias} alias`);
      }
    }
  }
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

test("Go CLI observes expiry in the real Lite store", async ({ request }) => {
  const before = await expiryStats(request);
  const plaintext = "real Lite expiry proof";
  const create = runRaw(["create", "--server", expiryServer, "--json"], plaintext);
  if (create.status !== 0 || create.stderr !== `burnerpad: server: ${expiryServer}\n`) {
    throw new Error("expiry fixture CLI create failed");
  }

  let created;
  try {
    created = JSON.parse(create.stdout);
  } catch {
    throw new Error("expiry fixture CLI create returned invalid JSON");
  }
  const receiptKeys = "link,mgmt_token,phrase,server,status,ttl";
  if (!created || created.status !== "created" || created.server !== expiryServer || created.ttl !== 5 ||
      typeof created.link !== "string" || typeof created.phrase !== "string" ||
      !managementTokenPattern.test(created.mgmt_token) || Object.keys(created).sort().join(",") !== receiptKeys) {
    throw new Error("expiry fixture CLI create returned an invalid receipt");
  }

  let createdURL;
  try {
    createdURL = new URL(created.link);
  } catch {
    throw new Error("expiry fixture CLI create returned an invalid link");
  }
  const createdID = createdURL.pathname.slice(3);
  if (createdURL.origin !== expiryServer || createdURL.pathname !== `/s/${createdID}` ||
      !canonicalIDPattern.test(createdID) || createdURL.search || createdURL.hash) {
    throw new Error("expiry fixture CLI create returned an invalid link");
  }
  if (await expiryPageStatus(request, created.link) !== 200) {
    throw new Error("expiry fixture row was not live after create");
  }

  await new Promise((resolve) => setTimeout(resolve, (created.ttl + 2) * 1000));

  const reveal = runRaw([
    "reveal", "--json", "--passphrase-file", protectedFile("expiry-phrase", created.phrase)
  ], created.link + "\n");
  const expectedStdout = JSON.stringify({
    status: "error",
    code: "secret_unavailable",
    message: "the secret is unavailable",
    server: expiryServer
  }) + "\n";
  const expectedStderr = `burnerpad: server: ${expiryServer}\n`;
  if (reveal.status !== 4 || reveal.stdout !== expectedStdout || reveal.stderr !== expectedStderr) {
    throw new Error("expiry fixture CLI reveal returned an unexpected result");
  }

  const after = await expiryStats(request);
  if (after.created !== before.created + 1 || after.expired !== before.expired + 1 ||
      after.claimed !== before.claimed || after.resident !== before.resident) {
    throw new Error("expiry fixture counters did not record exactly one expiry");
  }
});
