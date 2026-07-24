<template>
	<container label="Image">
		<template #options>
			<div class="flex flex-col gap-8px w-full">
				<div v-if="value" class="img-preview">
					<img :src="value" alt="preview" @error="onPreviewError" />
				</div>
				<n-input
					v-model:value="value"
					size="small"
					placeholder="Paste an image URL, or upload / pick from the library">
				</n-input>
				<div class="flex gap-8px">
					<n-button size="small" :loading="uploading" @click="pickFile">
						<i class="i-mdi:upload mr-4px"></i>Upload
					</n-button>
					<n-button size="small" @click="openLibrary">
						<i class="i-mdi:image-multiple-outline mr-4px"></i>Media Library
					</n-button>
				</div>
				<input
					ref="fileInput"
					type="file"
					accept="image/png,image/jpeg,image/gif,image/webp,image/bmp"
					class="hidden"
					@change="onFile" />
			</div>
		</template>
	</container>

	<n-modal
		v-model:show="libraryOpen"
		preset="card"
		title="Media Library"
		:bordered="false"
		style="width: 720px; max-width: 92vw">
		<div class="flex justify-between items-center mb-12px">
			<n-button size="small" type="primary" :loading="uploading" @click="pickFile">
				<i class="i-mdi:upload mr-4px"></i>Upload new
			</n-button>
			<span class="text-12px text-[var(--color-text-3)]">{{ items.length }} image(s)</span>
		</div>
		<n-spin :show="loading">
			<div v-if="items.length" class="media-grid">
				<div
					v-for="it in items"
					:key="it.name"
					class="media-cell"
					:class="{ selected: value === it.url }"
					@click="choose(it)">
					<img :src="it.url" :alt="it.name" loading="lazy" />
					<div class="media-actions">
						<n-button size="tiny" circle type="error" @click.stop="remove(it)">
							<i class="i-mdi:delete"></i>
						</n-button>
					</div>
				</div>
			</div>
			<div v-else class="media-empty">No images yet — click "Upload new" to add one.</div>
		</n-spin>
	</n-modal>
</template>

<script lang="ts" setup>
import { Message } from '@/utils'
import {
	uploadEmailMedia,
	listEmailMedia,
	deleteEmailMedia,
	MediaItem,
} from '@/api/modules/emailMedia'

import Container from './BaseContainer.vue'

const value = defineModel<string>('value')

const fileInput = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const libraryOpen = ref(false)
const loading = ref(false)
const items = ref<MediaItem[]>([])

const pickFile = () => fileInput.value?.click()

const onFile = async (e: Event) => {
	const input = e.target as HTMLInputElement
	const file = input.files?.[0]
	input.value = '' // allow re-selecting the same file
	if (!file) return
	uploading.value = true
	try {
		const res = (await uploadEmailMedia(file)) as any
		if (res && res.url) {
			value.value = res.url
			Message.success('Image uploaded')
			if (libraryOpen.value) await loadLibrary()
		}
	} finally {
		uploading.value = false
	}
}

const openLibrary = async () => {
	libraryOpen.value = true
	await loadLibrary()
}

const loadLibrary = async () => {
	loading.value = true
	try {
		const res = (await listEmailMedia()) as any
		items.value = res?.list || []
	} finally {
		loading.value = false
	}
}

const choose = (it: MediaItem) => {
	value.value = it.url
	libraryOpen.value = false
}

const remove = async (it: MediaItem) => {
	await deleteEmailMedia(it.name)
	if (value.value === it.url) value.value = ''
	await loadLibrary()
}

const onPreviewError = (e: Event) => {
	;(e.target as HTMLImageElement).style.opacity = '0.3'
}
</script>

<style lang="scss" scoped>
.hidden {
	display: none;
}

.img-preview {
	display: flex;
	align-items: center;
	justify-content: center;
	max-height: 120px;
	padding: 6px;
	border: 1px solid var(--color-border-1);
	border-radius: 4px;
	background:
		linear-gradient(45deg, #f0f0f0 25%, transparent 25%),
		linear-gradient(-45deg, #f0f0f0 25%, transparent 25%),
		linear-gradient(45deg, transparent 75%, #f0f0f0 75%),
		linear-gradient(-45deg, transparent 75%, #f0f0f0 75%);
	background-size: 12px 12px;
	background-position:
		0 0,
		0 6px,
		6px -6px,
		-6px 0;

	img {
		max-height: 108px;
		max-width: 100%;
		object-fit: contain;
	}
}

.media-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
	gap: 12px;
	max-height: 60vh;
	overflow: auto;
}

.media-cell {
	position: relative;
	aspect-ratio: 1;
	border: 2px solid var(--color-border-1);
	border-radius: 6px;
	overflow: hidden;
	cursor: pointer;
	transition: border-color 0.15s;

	&:hover {
		border-color: var(--color-primary-3);

		.media-actions {
			opacity: 1;
		}
	}

	&.selected {
		border-color: var(--color-primary-6, #2080f0);
		box-shadow: 0 0 0 2px rgba(32, 128, 240, 0.25);
	}

	img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.media-actions {
		position: absolute;
		top: 6px;
		right: 6px;
		opacity: 0;
		transition: opacity 0.15s;
	}
}

.media-empty {
	padding: 40px 0;
	text-align: center;
	color: var(--color-text-3);
	font-size: 13px;
}
</style>
