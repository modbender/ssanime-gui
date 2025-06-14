export const useCapitalize = (name: string) => {
  return name.charAt(0).toUpperCase() + name.slice(1);
};
