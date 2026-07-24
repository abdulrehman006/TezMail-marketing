import { RouteRecordRaw } from 'vue-router'
import { Layout } from '@/router/constant'

const route: RouteRecordRaw = {
	path: '/accounts',
	component: Layout,
	meta: {
		sort: 20,
		key: 'accounts',
		title: 'Accounts',
		titleKey: 'layout.menu.accounts',
		module: 'system',
	},
	children: [
		{
			path: '/accounts',
			name: 'Accounts',
			component: () => import('@/views/accounts/index.vue'),
		},
	],
}

export default route
