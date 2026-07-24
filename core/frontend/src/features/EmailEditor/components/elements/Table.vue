<template>
	<block-section
		:data="data"
		:cell-key="cellKey"
		:block-type="data.type"
		:block-index="blockIndex"
		@select="handleSelect"
		@unSelect="handleUnSelect">
		<table :style="tableStyle" class="et-table">
			<tbody>
				<tr v-for="(row, r) in rows" :key="r">
					<component
						:is="isHeader(r) ? 'th' : 'td'"
						v-for="(cell, c) in row"
						:key="c"
						:style="cellStyle(r)"
						contenteditable="true"
						spellcheck="false"
						@blur="commit(r, c, $event)"
						@mousedown.stop>
						{{ cell }}
					</component>
				</tr>
			</tbody>
		</table>
	</block-section>
</template>

<script lang="ts" setup>
import { useConfig } from '../../hooks/useConfig'
import { useStyle } from '../../hooks/useStyle'
import { BlockBaseType, BaseStyle } from '../../types/base'

import BlockSection from '../layout/Section.vue'

const { data } = defineProps({
	data: {
		type: Object as PropType<BlockBaseType>,
		required: true,
	},
	cellKey: {
		type: String,
		required: true,
	},
	blockIndex: {
		type: Number,
		required: true,
	},
})

const { blockConfigMap } = useConfig()
const { configToStyle } = useStyle()

const attr = computed(() => blockConfigMap.value[data.key].attr)
const rows = computed(() => attr.value.rows || [])

const isHeader = (r: number) => !!attr.value.tableHeader && r === 0

// Table-wide font/colour styling comes from the block style; borders collapse.
const tableStyle = computed(() => {
	const style = blockConfigMap.value[data.key].style as BaseStyle
	return {
		...configToStyle(style),
		width: '100%',
		borderCollapse: 'collapse' as const,
		tableLayout: 'fixed' as const,
	}
})

const cellStyle = (r: number) => {
	const a = attr.value
	const style: Record<string, string> = {
		border: `${a.borderWidth || '1px'} solid ${a.borderColor || '#dddddd'}`,
		padding: a.cellPadding || '8px',
		textAlign: 'left',
		verticalAlign: 'top',
		wordBreak: 'break-word',
	}
	if (isHeader(r)) {
		style.backgroundColor = a.headerBg || '#f4f4f4'
		style.fontWeight = 'bold'
	}
	return style
}

// Commit on blur so typing never triggers a re-render (avoids caret jumps).
const commit = (r: number, c: number, e: Event) => {
	const el = e.target as HTMLElement
	const value = el.innerText
	const current = rows.value
	if (!current[r] || current[r][c] === value) return
	const newRows = current.map(row => row.slice())
	newRows[r][c] = value
	blockConfigMap.value[data.key].attr = { ...attr.value, rows: newRows }
}

const handleSelect = () => {}
const handleUnSelect = () => {}
</script>

<style lang="scss" scoped>
.et-table {
	width: 100%;
	// Sit above the block-section overlay (z-index 100/101 in Section.vue) so
	// clicks reach the cells and inline editing works.
	position: relative;
	z-index: 102;

	td,
	th {
		outline: none;
		cursor: text;
		user-select: text;

		&:focus {
			box-shadow: inset 0 0 0 2px rgba(24, 144, 255, 0.4);
		}
	}
}
</style>
