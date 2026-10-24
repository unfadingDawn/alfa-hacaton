import { NextResponse, type NextRequest } from "next/server";

// Быстрая проверка наличия сессии до рендера защищённых страниц.
// Подпись токена проверяет backend; здесь — только перенаправление.
const protectedPrefixes = ["/home", "/feed", "/news", "/analytics", "/company", "/saved", "/settings", "/cabinet", "/support", "/onboarding", "/research"];

export function proxy(req: NextRequest) {
  const { pathname } = req.nextUrl;
  const hasSession = req.cookies.has("ai_access") || req.cookies.has("ai_refresh");
  if (protectedPrefixes.some((p) => pathname === p || pathname.startsWith(p + "/")) && !hasSession) {
    const url = req.nextUrl.clone();
    url.pathname = "/login";
    url.search = `?next=${encodeURIComponent(pathname)}`;
    return NextResponse.redirect(url);
  }
  if ((pathname === "/login" || pathname === "/register") && req.cookies.has("ai_access")) {
    const url = req.nextUrl.clone();
    url.pathname = "/home";
    url.search = "";
    return NextResponse.redirect(url);
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!api|_next/static|_next/image|favicon.ico|waves.jpg).*)"],
};
