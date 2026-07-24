import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/roles',
	component: Layout,
	meta: {
		sort: 21,
		key: 'roles',
		title: 'Roles',
		titleKey: 'layout.menu.roles',
		module: 'system',
	},
	children: [
		{
			path: '/roles',
			name: 'Roles',
			component: () => import('@/views/roles/index.vue'),
		},
	],
}

export default route
