import type { NextConfig } from "next";

// Все запросы /api/* проксируются в Go-backend: cookie остаются на одном
// домене (HttpOnly, SameSite=Lax), ключи API во frontend не попадают.
const backend = process.env.BACKEND_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  experimental: {
    // Полный анализ агентом может занимать до нескольких минут.
    proxyTimeout: 330_000,
  },
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${backend}/api/:path*` }];
  },
};

export default nextConfig;
