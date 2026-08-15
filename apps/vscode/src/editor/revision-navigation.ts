export async function openDocumentIfCurrent<T>(
  open: () => PromiseLike<T>,
  show: (document: T) => PromiseLike<void>,
  canShow: (document: T) => boolean,
  isCurrentAfterShow: (document: T) => boolean,
): Promise<T | undefined> {
  const document = await open()
  if (!canShow(document)) return undefined
  await show(document)
  return isCurrentAfterShow(document) ? document : undefined
}
