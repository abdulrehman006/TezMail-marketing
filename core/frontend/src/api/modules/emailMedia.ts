import { AxiosProgressEvent } from 'axios'
import { instance } from '@/api'

export interface MediaItem {
	name: string
	url: string
	size: number
	update_time?: number
}

/**
 * @description Upload one image to the shared email-media library.
 * Resolves to the stored { name, url, size }.
 */
export const uploadEmailMedia = (file: File, onProgress?: (percent: number) => void) => {
	const form = new FormData()
	form.append('file', file)
	return instance.post('/email-media/upload', form, {
		headers: { 'Content-Type': 'multipart/form-data' },
		onUploadProgress: (e: AxiosProgressEvent) => {
			if (e.total) onProgress?.(Math.round((e.loaded / e.total) * 100))
		},
	})
}

/**
 * @description List uploaded email images (newest first).
 */
export const listEmailMedia = () => {
	return instance.get('/email-media/list')
}

/**
 * @description Delete an uploaded image by its stored name.
 */
export const deleteEmailMedia = (name: string) => {
	return instance.post(
		'/email-media/delete',
		{ name },
		{ fetchOptions: { successMessage: true } }
	)
}
