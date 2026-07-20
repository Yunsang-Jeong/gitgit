<script lang="ts">
  export let name = 'remote'
  export let size = 14
</script>

{#if name === 'github'}
  <svg class="remote-badge-icon github" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path d="M12 .7a11.3 11.3 0 0 0-3.57 22.03c.56.1.77-.24.77-.54v-2.1c-3.13.68-3.79-1.51-3.79-1.51-.51-1.3-1.25-1.65-1.25-1.65-1.02-.7.08-.69.08-.69 1.13.08 1.72 1.16 1.72 1.16 1 1.72 2.63 1.22 3.27.93.1-.73.39-1.22.71-1.5-2.5-.29-5.14-1.25-5.14-5.58 0-1.23.44-2.24 1.16-3.03-.12-.28-.5-1.43.11-2.99 0 0 .95-.3 3.1 1.16a10.8 10.8 0 0 1 5.64-.01c2.15-1.46 3.09-1.16 3.09-1.16.62 1.56.23 2.71.12 2.99.72.79 1.16 1.8 1.16 3.03 0 4.34-2.64 5.29-5.16 5.57.4.35.77 1.04.77 2.1v3.11c0 .3.2.65.78.54A11.3 11.3 0 0 0 12 .7Z" />
  </svg>
{:else if name === 'gitlab'}
  <svg class="remote-badge-icon gitlab" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path fill="#e24329" d="M12 22 1 14.1l2.2-6.8h17.6l2.2 6.8Z" />
    <path fill="#fc6d26" d="M12 22 3.2 7.3h5.1Zm0 0 3.7-14.7h5.1Z" />
    <path fill="#fca326" d="M8.3 7.3h7.4L12 22Z" />
    <path fill="#e24329" d="M3.2 7.3 5.3.9l3 6.4Zm12.5 0 3-6.4 2.1 6.4Z" />
  </svg>
{:else if name === 'bitbucket'}
  <svg class="remote-badge-icon bitbucket" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path d="M2.2 3.2a1 1 0 0 1 1-.9h17.6a1 1 0 0 1 1 .9l-2.5 17a1.5 1.5 0 0 1-1.5 1.3H6.2a1.5 1.5 0 0 1-1.5-1.3Zm11.9 12.2 1-6.8H8.9l1 6.8Z" />
  </svg>
{:else if name === 'azure-devops'}
  <svg class="remote-badge-icon azure-devops" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path d="M2 6.2 8.3 2v3.1l7.9-2.4L22 6.9v10.4l-5.8 4-8-2.4V22L2 17.8Zm6.3 1.2v9.2l7.9 1.6V5.8Z" />
  </svg>
{:else if name === 'codeberg'}
  <svg class="remote-badge-icon codeberg" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path class="codeberg-sky" d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Z" />
    <path class="codeberg-mountain" d="m4.1 17.7 7.7-12.6 8.1 12.6-4.8-3.1-3.3-5.4-3.2 5.3Z" />
  </svg>
{:else if name === 'gitea'}
  <svg class="remote-badge-icon gitea" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path d="M3 5.2h14.8v3H21v5.2a3.8 3.8 0 0 1-3.8 3.8H16A6.8 6.8 0 0 1 9.8 21 6.8 6.8 0 0 1 3 14.2Zm14.8 5.5v4h.2a1.8 1.8 0 0 0 1.8-1.8v-2.2Z" />
    <path class="gitea-branch" d="M8.2 8.4a1.4 1.4 0 1 0 0 2.8 1.4 1.4 0 0 0 0-2.8Zm4.1 4.2a1.4 1.4 0 1 0 0 2.8 1.4 1.4 0 0 0 0-2.8Zm-3.1-2.1v2.1c0 1 .8 1.8 1.8 1.8h.2" />
  </svg>
{:else if name === 'git'}
  <svg class="remote-badge-icon git" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path d="m22.3 10.6-8.9-8.9a2 2 0 0 0-2.8 0L8.8 3.5l2.3 2.3a2.4 2.4 0 0 1 3 3l2.2 2.2a2.4 2.4 0 1 1-1.4 1.4l-2-2v5.1a2.4 2.4 0 1 1-2 0V10a2.4 2.4 0 0 1-1.3-3L7.4 4.9l-5.7 5.7a2 2 0 0 0 0 2.8l8.9 8.9a2 2 0 0 0 2.8 0l8.9-8.9a2 2 0 0 0 0-2.8Z" />
  </svg>
{:else if name === 'cloud'}
  <svg class="remote-badge-icon cloud" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path d="M7.2 19a5.2 5.2 0 0 1-.8-10.3A6.6 6.6 0 0 1 19 10.5 4.3 4.3 0 0 1 18.3 19Z" />
  </svg>
{:else if name === 'server'}
  <svg class="remote-badge-icon server" style:width={`${size}px`} style:height={`${size}px`} viewBox="0 0 24 24" aria-hidden="true">
    <path d="M3 3h18v7H3Zm0 11h18v7H3Zm3-7.5a1 1 0 1 0 0-2 1 1 0 0 0 0 2Zm0 11a1 1 0 1 0 0-2 1 1 0 0 0 0 2Zm3-12h9v1H9Zm0 11h9v1H9Z" />
  </svg>
{:else if name === 'remote'}
  <span class="remote-badge-icon remote" style:width={`${size}px`} style:height={`${size}px`} aria-hidden="true">R</span>
{:else}
  <span class="remote-badge-icon custom" style:width={`${size}px`} style:height={`${size}px`} title={name} aria-hidden="true">{name}</span>
{/if}

<style>
  .remote-badge-icon {
    display: inline-flex;
    flex: 0 0 auto;
    align-items: center;
    justify-content: center;
    overflow: visible;
    fill: currentColor;
    color: #9eb0b8;
  }

  .github { color: #f0f4f6; }
  .bitbucket { color: #2684ff; }
  .azure-devops { color: #1689d4; }
  .git { color: #f05133; }
  .gitea { color: #609926; }
  .gitea-branch { fill: none; stroke: white; stroke-linecap: round; stroke-width: 1.2; }
  .codeberg-sky { fill: #2185d0; }
  .codeberg-mountain { fill: #ffffff; }
  .cloud, .server { color: #9fb0b7; }
  span.remote {
    border: 1px solid currentColor;
    border-radius: 50%;
    font-size: 8px;
    font-weight: 750;
    line-height: 1;
  }
  span.custom {
    overflow: hidden;
    font-size: 10px;
    line-height: 1;
    text-overflow: clip;
    white-space: nowrap;
  }
</style>
