// Only this regression page imports the mock runtime; no calls reach the database.
export const handlers = new Map();
export const Call = { ByName(name, ...args) {
  const method = name.slice(name.lastIndexOf(".") + 1);
  return Promise.resolve().then(() => handlers.get(method)?.(...args));
} };
export const Events = { On() {} };
