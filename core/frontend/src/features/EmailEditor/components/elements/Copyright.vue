<template>
	<component v-if="copyrightVNode" :is="copyrightVNode"></component>
</template>

<script lang="ts" setup>
import { copyrightVNode } from '../../config/config'
</script>
