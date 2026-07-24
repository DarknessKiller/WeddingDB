<script lang="ts">
  import { Monitor, Save, Bell, Shield } from 'lucide-svelte';
</script>

<svelte:head><title>Settings – WeddingDB</title></svelte:head>

<div class="p-4 sm:p-7 max-w-2xl mx-auto">
  <h1 class="text-xl font-bold text-gray-900 mb-6">Settings</h1>

  <div class="space-y-4">
    <div class="bg-white border border-gray-200 rounded-2xl p-4 sm:p-5 flex flex-col sm:flex-row items-start sm:items-center gap-3 sm:gap-4">
      <div class="w-10 h-10 rounded-xl bg-red-50 flex items-center justify-center text-red"><Monitor class="w-5 h-5" /></div>
      <div class="flex-1">
        <div class="font-semibold text-gray-900 text-sm">Kiosk Mode Display</div>
        <div class="text-xs text-gray-500">Configure the reception kiosk screen</div>
      </div>
      <button class="w-full sm:w-auto px-4 py-2 text-sm border border-gray-200 rounded-xl hover:bg-gray-50">Configure</button>
    </div>

    <div class="bg-white border border-gray-200 rounded-2xl p-4 sm:p-5 flex flex-col sm:flex-row items-start sm:items-center gap-3 sm:gap-4">
      <div class="w-10 h-10 rounded-xl bg-gold-50 flex items-center justify-center text-gold"><Bell class="w-5 h-5" /></div>
      <div class="flex-1">
        <div class="font-semibold text-gray-900 text-sm">Notifications</div>
        <div class="text-xs text-gray-500">Check-in alerts and reminders</div>
      </div>
      <button class="w-full sm:w-auto px-4 py-2 text-sm border border-gray-200 rounded-xl hover:bg-gray-50">Configure</button>
    </div>

    <div class="bg-white border border-gray-200 rounded-2xl p-4 sm:p-5 flex flex-col sm:flex-row items-start sm:items-center gap-3 sm:gap-4">
      <div class="w-10 h-10 rounded-xl bg-emerald-50 flex items-center justify-center text-emerald-700"><Shield class="w-5 h-5" /></div>
      <div class="flex-1">
        <div class="font-semibold text-gray-900 text-sm">Data Management</div>
        <div class="text-xs text-gray-500">Export, import, and backup guest data</div>
      </div>
      <button class="w-full sm:w-auto px-4 py-2 text-sm border border-gray-200 rounded-xl hover:bg-gray-50">Configure</button>
    </div>
  </div>
</div>
