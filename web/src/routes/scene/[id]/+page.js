// A scene id is a row in the server's database, so there is nothing to
// prerender here. The Go server answers /app/scene/<id> with the app shell
// (index.html), and the client router renders this page from it.
export const prerender = false;
