// ghosts.xamjiang.com
//
//   /             landing page
//   /install      the install script for curl, the landing page for browsers
//   /install.sh   the install script, always (for reading it first)
//   /apt/...      the APT repository, from R2
import script from "../../packaging/install.sh";
import page from "./page.html";

const REPO = "https://github.com/xamjiang/ghosts";

export default {
	async fetch(request, env) {
		const url = new URL(request.url);
		if (request.method !== "GET" && request.method !== "HEAD") {
			return new Response("Method not allowed", { status: 405, headers: { Allow: "GET, HEAD" } });
		}
		const path = url.pathname;
		if (path === "/" || (path === "/install" && wantsHTML(request))) {
			return html(page);
		}
		if (path === "/install" || path === "/install.sh") {
			return new Response(script, {
				headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "public, max-age=300" },
			});
		}
		if (path === "/apt" || path === "/apt/") {
			return Response.redirect(`${url.origin}/#apt`, 302);
		}
		if (path.startsWith("/apt/")) {
			return apt(request, env, path.slice(1));
		}
		if (path === "/github") {
			return Response.redirect(REPO, 302);
		}
		return new Response("Not found\n", { status: 404 });
	},
};

// curl and apt send "Accept: */*"; browsers ask for text/html first.
function wantsHTML(request) {
	return (request.headers.get("Accept") || "").includes("text/html");
}

function html(body) {
	return new Response(body, {
		headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "public, max-age=300" },
	});
}

const TYPES = {
	".deb": "application/vnd.debian.binary-package",
	".gz": "application/gzip",
	".gpg": "application/pgp-keys",
	".asc": "application/pgp-keys",
};

async function apt(request, env, key) {
	if (key.includes("..")) {
		return new Response("Not found\n", { status: 404 });
	}
	const obj = await env.BUCKET.get(key, { onlyIf: request.headers, range: request.headers });
	if (obj === null) {
		return new Response("Not found\n", { status: 404 });
	}
	const headers = new Headers();
	obj.writeHttpMetadata(headers);
	headers.set("ETag", obj.httpEtag);
	headers.set("Last-Modified", obj.uploaded.toUTCString());
	headers.set("Accept-Ranges", "bytes");
	const ext = key.slice(key.lastIndexOf("."));
	headers.set("Content-Type", TYPES[ext] || "text/plain; charset=utf-8");
	// Packages are immutable once in the pool; indexes change every release.
	headers.set("Cache-Control", key.startsWith("apt/pool/") ? "public, max-age=31536000, immutable" : "public, max-age=60");

	if (!("body" in obj)) {
		return new Response(null, { status: 304, headers }); // a precondition matched
	}
	if (request.headers.has("Range") && obj.range && "offset" in obj.range) {
		const end = obj.range.offset + obj.range.length - 1;
		headers.set("Content-Range", `bytes ${obj.range.offset}-${end}/${obj.size}`);
		return new Response(request.method === "HEAD" ? null : obj.body, { status: 206, headers });
	}
	headers.set("Content-Length", String(obj.size));
	return new Response(request.method === "HEAD" ? null : obj.body, { headers });
}
