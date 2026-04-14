const TELEGRAM_API_BASE_URL = "https://api.telegram.org";

function json(status, payload) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}

function extractBotPath(pathname) {
  const match = pathname.match(/^\/bot([^/]+)(\/.*)?$/);
  if (!match) {
    return null;
  }
  return {
    token: match[1],
    suffix: match[2] || "",
  };
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const parsed = extractBotPath(url.pathname);
    if (!parsed) {
      return json(404, { ok: false, error: "not_found" });
    }

    const expectedToken = String(env.TELEGRAM_BOT_TOKEN || "").trim();
    if (!expectedToken) {
      return json(500, {
        ok: false,
        error: "relay_secret_not_configured",
      });
    }

    // Do not operate as a generic public relay. We only proxy requests that
    // target the bot token explicitly configured for this worker.
    if (parsed.token !== expectedToken) {
      return json(403, {
        ok: false,
        error: "forbidden",
      });
    }

    const upstreamURL = new URL(
      `${TELEGRAM_API_BASE_URL}/bot${expectedToken}${parsed.suffix}${url.search}`,
    );

    const headers = new Headers(request.headers);
    headers.delete("host");
    headers.delete("cf-connecting-ip");
    headers.delete("x-forwarded-for");
    headers.delete("x-real-ip");

    const init = {
      method: request.method,
      headers,
      body:
        request.method === "GET" || request.method === "HEAD"
          ? undefined
          : request.body,
      redirect: "manual",
    };

    try {
      const upstreamResponse = await fetch(upstreamURL, init);
      const responseHeaders = new Headers(upstreamResponse.headers);
      responseHeaders.delete("content-encoding");
      responseHeaders.delete("content-length");

      return new Response(upstreamResponse.body, {
        status: upstreamResponse.status,
        statusText: upstreamResponse.statusText,
        headers: responseHeaders,
      });
    } catch (error) {
      return json(502, {
        ok: false,
        error: "upstream_fetch_failed",
        message: error instanceof Error ? error.message : String(error),
      });
    }
  },
};
