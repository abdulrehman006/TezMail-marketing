<template>
	<Collapse :default-expanded-names="['0', '1', '2']">
		<n-collapse-item title="Table" name="0">
			<div class="py-8px flex flex-col gap-12px">
				<div class="flex items-center justify-between">
					<span class="text-13px">Rows: {{ rowCount }}</span>
					<div class="flex gap-6px">
						<n-button size="tiny" @click="addRow">+ Row</n-button>
						<n-button size="tiny" :disabled="rowCount <= 1" @click="removeRow">- Row</n-button>
					</div>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-13px">Columns: {{ colCount }}</span>
					<div class="flex gap-6px">
						<n-button size="tiny" @click="addCol">+ Col</n-button>
						<n-button size="tiny" :disabled="colCount <= 1" @click="removeCol">- Col</n-button>
					</div>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-13px">Header row</span>
					<n-switch v-model:value="attr.tableHeader" />
				</div>
				<div class="flex items-center justify-between">
					<span class="text-13px">Header background</span>
					<n-color-picker
						v-model:value="attr.headerBg"
						:show-alpha="false"
						:modes="['hex']"
						class="w-120px">
					</n-color-picker>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-13px">Border color</span>
					<n-color-picker
						v-model:value="attr.borderColor"
						:show-alpha="false"
						:modes="['hex']"
						class="w-120px">
					</n-color-picker>
				</div>
				<div class="flex items-center justify-between">
					<span class="text-13px">Border width</span>
					<n-input v-model:value="attr.borderWidth" size="small" placeholder="1px" class="w-120px" />
				</div>
				<div class="flex items-center justify-between">
					<span class="text-13px">Cell padding</span>
					<n-input v-model:value="attr.cellPadding" size="small" placeholder="8px" class="w-120px" />
				</div>
				<div class="text-12px text-[var(--color-text-3)]">Click a cell in the canvas to edit its text.</div>
			</div>
		</n-collapse-item>
		<n-collapse-item title="Text" name="1">
			<div class="py-8px">
				<TextForm v-model:value="blockStyle" />
			</div>
		</n-collapse-item>
		<n-collapse-item title="Container" name="2">
			<div class="py-8px">
				<ContainerForm v-model:value="containerStyle" />
			</div>
		</n-collapse-item>
	</Collapse>
</template>

<script lang="tsx" setup>
import { useConfig } from '../../hooks/useConfig'
import { useNormalForm } from '../style/useNormalForm'
import { useSetData } from '../../hooks/useSetData'

import Collapse from '../shared/Collapse.vue'

const { selectedBlockKey, blockConfigMap } = useConfig()
const { autoSaveFn } = useSetData()

const attr = computed(() => blockConfigMap.value[selectedBlockKey.value].attr)

const blockStyle = computed({
	get() {
		return blockConfigMap.value[selectedBlockKey.value].style
	},
	set(newVal) {
		blockConfigMap.value[selectedBlockKey.value].style = newVal
	},
})

const containerStyle = computed({
	get() {
		return blockConfigMap.value[selectedBlockKey.value].containerStyle
	},
	set(newVal) {
		blockConfigMap.value[selectedBlockKey.value].containerStyle = newVal
	},
})

const rows = computed<string[][]>(() => attr.value.rows || [])
const rowCount = computed(() => rows.value.length)
const colCount = computed(() => rows.value[0]?.length || 0)

const setRows = (newRows: string[][]) => {
	blockConfigMap.value[selectedBlockKey.value].attr = { ...attr.value, rows: newRows }
}

const addRow = () => {
	const cols = colCount.value || 1
	setRows([...rows.value.map(r => r.slice()), new Array(cols).fill('Cell')])
}
const removeRow = () => {
	if (rowCount.value <= 1) return
	setRows(rows.value.slice(0, -1).map(r => r.slice()))
}
const addCol = () => {
	setRows(rows.value.map(r => [...r, 'Cell']))
}
const removeCol = () => {
	if (colCount.value <= 1) return
	setRows(rows.value.map(r => r.slice(0, -1)))
}

watch(
	() => [attr.value, blockStyle.value, containerStyle.value],
	() => {
		autoSaveFn()
	},
	{ deep: true }
)

const [TextForm] = useNormalForm([
	{ attrKey: 'fontWeight' },
	{ attrKey: 'fontSize' },
	{ attrKey: 'color', label: 'Text Color' },
	{ attrKey: 'lineHeight' },
	{ attrKey: 'letterSpacing' },
])

const [ContainerForm] = useNormalForm([{ attrKey: 'textAlign' }, { attrKey: 'padding' }])
</script>
