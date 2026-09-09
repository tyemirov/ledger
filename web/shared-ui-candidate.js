// @ts-check
import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

export const revision = "bbc21cc264d96b51195e0c1a264ad43f14a205ad";
const assets = {
  "mpr-ui-config.js": "3f56fbd212a516d2bd8b0b95f73ae7ad82952c10d8d5f4e6f8b44d3233f01304",
  "mpr-ui.js": "3e725dbe911470ca934cb46456369479b6ac232eee5ccba2582bf8d939259ae8",
  "mpr-ui.css": "31b92536df3a1584b7f19ac50eb61d6c7aff7c710ee92b84c46835194849e816",
  "js-yaml.min.js": "45dc3dd03dc07a06705a2c2989b8c7f709013f04bd5386e3279d4e447f07ebd7",
};
const directory = path.join(import.meta.dirname, "node_modules/.mpr-ui-candidate", revision);
export async function prepareSharedUI() {
  await mkdir(directory, { recursive: true });
  for (const [name, digest] of Object.entries(assets)) {
    const destination = path.join(directory, name);
    let bytes;
    try { bytes = await readFile(destination); }
    catch (error) {
      if (error.code !== "ENOENT") throw error;
      const url = name === "js-yaml.min.js"
        ? "https://cdn.jsdelivr.net/npm/js-yaml@4.1.0/dist/js-yaml.min.js"
        : `https://raw.githubusercontent.com/MarcoPoloResearchLab/mpr-ui/${revision}/${name}`;
      const response = await fetch(url, { signal: AbortSignal.timeout(15_000) });
      if (!response.ok) throw new Error(`shared_ui_download:${name}:${response.status}`);
      bytes = Buffer.from(await response.arrayBuffer());
    }
    if (createHash("sha256").update(bytes).digest("hex") !== digest) throw new Error(`shared_ui_digest:${name}`);
    await writeFile(destination, bytes);
  }
}
export async function installSharedUI(context) {
  for (const name of Object.keys(assets)) {
    const url = name === "js-yaml.min.js"
      ? "https://cdn.jsdelivr.net/npm/js-yaml@4.1.0/dist/js-yaml.min.js"
      : `https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/${name}`;
    const body = await readFile(path.join(directory, name));
    await context.route(`${url}*`, route => route.fulfill({ body, contentType: name.endsWith(".css") ? "text/css" : "text/javascript" }));
  }
  const body = await readFile(path.join(import.meta.dirname, "google-identity-fixture.js"));
  await context.route("https://accounts.google.com/gsi/client", route => route.fulfill({ body, contentType: "text/javascript" }));
}
