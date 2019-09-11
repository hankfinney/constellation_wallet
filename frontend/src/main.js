import Vue from "vue";
import App from "./App.vue";
import router from "./router/index";

Vue.config.productionTip = false;
Vue.config.devtools = true;

import Wails from '@wailsapp/runtime';
import PaperDashboard from "./plugins/paperDashboard";
import "vue-notifyjs/themes/default.css";
import VueNotify from 'vue-notifyjs'
import VueSweetalert2 from 'vue-sweetalert2';
import {store} from './store/store'

Vue.use(VueNotify);
Vue.use(PaperDashboard);
Vue.use(VueSweetalert2);

Wails.Init(() => {
  new Vue({
    router,
    store: store,
    render: h => h(App)
  }).$mount("#app");
});
