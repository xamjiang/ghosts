// ghosts.xamjiang.com
//
//   /             landing page
//   /images/…     the README screenshots the landing page shows
//   /install      the install script for curl, the landing page for browsers
//   /install.sh   the install script, always (for reading it first)
//   /apt/...      the APT repository, served from the latest GitHub release
import script from "../../packaging/install.sh";
import keyAsc from "../../packaging/apt-key.asc";
import keyGpg from "../../packaging/apt-key.gpg";
import page from "./page.html";
import demo from "../../docs/images/demo.svg";
import detached from "../../docs/images/detached.svg";
import power from "../../docs/images/power.svg";
import containers from "../../docs/images/containers.svg";
import stop from "../../docs/images/stop.svg";

const images = {
	"demo.svg": demo,
	"detached.svg": detached,
	"power.svg": power,
	"containers.svg": containers,
	"stop.svg": stop,
};

const REPO = "https://github.com/xamjiang/ghosts";

export default {
	async fetch(request, env, ctx) {
		const url = new URL(request.url);
		if (request.method !== "GET" && request.method !== "HEAD") {
			return new Response("Method not allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
		}
		const path = decodeURIComponent(url.pathname);
		if (path === "/" || (path === "/install" && wantsHTML(request))) {
			return html(page);
		}
		if (path.startsWith("/images/") && Object.hasOwn(images, path.slice("/images/".length))) {
			return text(images[path.slice("/images/".length)], "image/svg+xml; charset=utf-8", 3600);
		}
		if (path === "/install" || path === "/install.sh") {
			return text(script, "text/plain; charset=utf-8", 300);
		}
		if (path === "/apt" || path === "/apt/") {
			return Response.redirect(`${url.origin}/#apt`, 302);
		}
		if (path.startsWith("/apt/")) {
			return apt(path.slice("/apt/".length), env, ctx);
		}
		if (path === "/github") {
			return Response.redirect(REPO, 302);
		}
		return notFound();
	},
};

// curl and apt send "Accept: */*"; browsers ask for text/html first.
function wantsHTML(request) {
	return (request.headers.get("Accept") || "").includes("text/html");
}

function html(body) {
	return text(body, "text/html; charset=utf-8", 300);
}

function text(body, type, maxAge) {
	return new Response(body, { headers: { "Content-Type": type, "Cache-Control": `public, max-age=${maxAge}` } });
}

function notFound() {
	return new Response("Not found\n", { status: 404 });
}

// The repository has no storage of its own. Each release carries its signed
// index as flat assets (apt_InRelease, apt_Packages_amd64.gz, …) next to its
// .deb files, so:
//
//   key.gpg, key.asc                     bundled into this Worker
//   dists/stable/…                       the index assets of the latest release
//   pool/main/g/ghosts/ghosts_V_A.deb    a redirect to release vV's .deb for A
async function apt(path, env, ctx) {
	if (path === "key.gpg" || path === "key.asc") {
		return new Response(path === "key.gpg" ? keyGpg : keyAsc, {
			headers: { "Content-Type": "application/pgp-keys", "Cache-Control": "public, max-age=86400" },
		});
	}

	const deb = path.match(/^pool\/main\/g\/ghosts\/ghosts_([^_/]+)_(amd64|arm64)\.deb$/);
	if (deb) {
		// packaging/deb.sh turned the tag's "-" into "~" (0.6.0-rc1 → 0.6.0~rc1).
		const tag = "v" + deb[1].replaceAll("~", "-");
		return Response.redirect(`${env.RELEASES}/download/${tag}/ghosts_linux_${deb[2]}.deb`, 302);
	}

	const asset = indexAsset(path);
	if (!asset) {
		return notFound();
	}
	const tag = await latestTag(env, ctx);
	if (!tag) {
		return new Response("Couldn't find the latest release\n", { status: 502 });
	}
	// Assets of a given tag don't change, so cache them at the edge; a 404 (a
	// release without an index) only briefly.
	const res = await fetch(`${env.RELEASES}/download/${tag}/${asset}`, {
		cf: { cacheEverything: true, cacheTtlByStatus: { "200-299": 86400, 404: 30, "500-599": 0 } },
	});
	if (res.status === 404) {
		return notFound();
	}
	if (!res.ok) {
		return new Response("Upstream error\n", { status: 502 });
	}
	return new Response(res.body, {
		headers: {
			"Content-Type": asset.endsWith(".gz") ? "application/gzip" : "text/plain; charset=utf-8",
			"Cache-Control": "public, max-age=60",
		},
	});
}

// indexAsset maps a dists/ path to the release asset holding it.
function indexAsset(path) {
	const top = path.match(/^dists\/stable\/(InRelease|Release|Release\.gpg)$/);
	if (top) {
		return `apt_${top[1]}`;
	}
	const pkgs = path.match(/^dists\/stable\/main\/binary-(amd64|arm64)\/Packages(\.gz)?$/);
	if (pkgs) {
		return `apt_Packages_${pkgs[1]}${pkgs[2] || ""}`;
	}
	return null;
}

// latestTag resolves releases/latest at most once a minute per location, so
// the index files one `apt update` fetches all come from the same release
// even while a new one is being published.
async function latestTag(env, ctx) {
	const key = new Request("https://ghosts.xamjiang.com/_internal/latest-tag");
	const hit = await caches.default.match(key);
	if (hit) {
		return hit.text();
	}
	const res = await fetch(`${env.RELEASES}/latest`, { redirect: "manual" });
	const m = (res.headers.get("Location") || "").match(/\/releases\/tag\/([^/?#]+)$/);
	if (!m) {
		return null;
	}
	const tag = decodeURIComponent(m[1]);
	ctx.waitUntil(caches.default.put(key, new Response(tag, { headers: { "Cache-Control": "public, max-age=60" } })));
	return tag;
}
