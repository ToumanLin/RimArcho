// Only this regression page imports the mock runtime; no calls reach the database.
export const handlers = new Map();
export const Call = { ByName(name, ...args) {
  const method = name.slice(name.lastIndexOf(".") + 1);
  return Promise.resolve().then(async () => {
    const result = await handlers.get(method)?.(...args);
    if (method === "Workspace") {
      emit("environment", { request: args[0], ...result });
      return true;
    }
    return result;
  });
} };
const subscribers = new Map();
export const Events = { On(name, fn) {
  if (!subscribers.has(name)) subscribers.set(name, []);
  subscribers.get(name).push(fn);
} };
export function emit(name, data) { for (const fn of subscribers.get(name) || []) fn({ data }); }
