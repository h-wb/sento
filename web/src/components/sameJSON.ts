// sameJSON compares two form values by their JSON encoding.
export const sameJSON = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b)
