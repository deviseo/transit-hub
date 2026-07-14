#!/usr/bin/env node
import { mkdir, readFile, rename, unlink, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';

const WIDTH = 960;
const HEIGHT = 480;
const DAY_MS = 86_400_000;
const PADDING = { top: 58, right: 42, bottom: 72, left: 76 };
const API_VERSION = '2026-03-10';
const DEFAULT_REPOSITORY = 'deviseo/transit-hub';
const OUTPUTS = [
  { theme: 'light', file: 'docs/assets/star-history-light.svg' },
  { theme: 'dark', file: 'docs/assets/star-history-dark.svg' },
];

const THEMES = {
  light: {
    background: '#ffffff',
    foreground: '#111827',
    muted: '#6b7280',
    grid: '#e5e7eb',
    axis: '#9ca3af',
    line: '#2563eb',
    area: '#bfdbfe',
    areaOpacity: '0.58',
  },
  dark: {
    background: '#0b1020',
    foreground: '#f8fafc',
    muted: '#cbd5e1',
    grid: '#263247',
    axis: '#64748b',
    line: '#60a5fa',
    area: '#1d4ed8',
    areaOpacity: '0.38',
  },
};

const repository = process.env.GITHUB_REPOSITORY || DEFAULT_REPOSITORY;
const token = process.env.GITHUB_TOKEN || process.env.GH_TOKEN;

main().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});

async function main() {
  const { owner, repo } = parseRepository(repository);
  if (!token) {
    throw new Error('GITHUB_TOKEN or GH_TOKEN is required to fetch authenticated stargazer timestamps.');
  }

  const stars = await fetchAllStargazerTimes(owner, repo, token);

  for (const output of OUTPUTS) {
    const svg = renderSvg({
      repository: `${owner}/${repo}`,
      stars,
      themeName: output.theme,
    });
    await atomicWrite(resolve(output.file), svg);
  }

  console.log(`Generated ${OUTPUTS.length} star history SVGs for ${owner}/${repo} with ${stars.length} stars.`);
}

function parseRepository(value) {
  const [owner, repo, extra] = value.split('/');
  if (!owner || !repo || extra) {
    throw new Error(`Invalid GITHUB_REPOSITORY value: ${value}`);
  }
  return { owner, repo };
}

async function fetchAllStargazerTimes(owner, repo, authToken) {
  const timestamps = new Set();

  for (let page = 1; ; page += 1) {
    const url = new URL(`https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/stargazers`);
    url.searchParams.set('per_page', '100');
    url.searchParams.set('page', String(page));

    const response = await fetch(url, {
      headers: {
        Accept: 'application/vnd.github.star+json',
        Authorization: `Bearer ${authToken}`,
        'User-Agent': 'transit-hub-star-history-generator',
        'X-GitHub-Api-Version': API_VERSION,
      },
    });

    if (!response.ok) {
      const body = await response.text();
      const detail = body.trim().slice(0, 500) || response.statusText;
      throw new Error(`GitHub stargazers request failed on page ${page}: HTTP ${response.status} ${detail}`);
    }

    const payload = await response.json();
    if (!Array.isArray(payload)) {
      throw new Error(`GitHub stargazers request returned a non-array payload on page ${page}.`);
    }

    for (const item of payload) {
      if (!item || typeof item.starred_at !== 'string') {
        continue;
      }
      const date = new Date(item.starred_at);
      if (!Number.isNaN(date.getTime())) {
        timestamps.add(date.toISOString());
      }
    }

    if (payload.length < 100) {
      break;
    }
  }

  return [...timestamps].sort();
}

function renderSvg({ repository, stars, themeName }) {
  const theme = THEMES[themeName];
  const title = `${repository} Star History`;
  const starCount = stars.length;
  const firstTime = stars[0] ? new Date(stars[0]).getTime() : Date.UTC(2026, 0, 1);
  const lastTime = stars.at(-1) ? new Date(stars.at(-1)).getTime() : firstTime + 86_400_000;
  const minTime = firstTime === lastTime ? firstTime - 43_200_000 : firstTime;
  const maxTime = firstTime === lastTime ? lastTime + 43_200_000 : lastTime;
  const maxStars = Math.max(starCount, 1);
  const plot = {
    x: PADDING.left,
    y: PADDING.top,
    width: WIDTH - PADDING.left - PADDING.right,
    height: HEIGHT - PADDING.top - PADDING.bottom,
  };

  const xFor = (time) => plot.x + ((time - minTime) / (maxTime - minTime || 1)) * plot.width;
  const yFor = (count) => plot.y + plot.height - (count / maxStars) * plot.height;
  const points = stars.map((timestamp, index) => `${round(xFor(new Date(timestamp).getTime()))},${round(yFor(index + 1))}`);
  const linePath = points.length > 0 ? `M ${points.join(' L ')}` : `M ${plot.x},${plot.y + plot.height}`;
  const areaPath = points.length > 0
    ? `${linePath} L ${round(xFor(lastTime))},${plot.y + plot.height} L ${round(xFor(firstTime))},${plot.y + plot.height} Z`
    : '';
  const xTicks = createDateTicks(minTime, maxTime, 5);
  const yTicks = createNumberTicks(maxStars, 5);
  const latestLabel = stars.at(-1) ? formatDate(new Date(stars.at(-1))) : 'No stars yet';
  const formatXAxisLabel = createXAxisFormatter(maxTime - minTime);

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${HEIGHT}" viewBox="0 0 ${WIDTH} ${HEIGHT}" role="img" aria-labelledby="title desc">
  <title id="title">${escapeXml(title)}</title>
  <desc id="desc">Cumulative GitHub stars over time for ${escapeXml(repository)}. Current star count: ${starCount}. Latest star date: ${escapeXml(latestLabel)}.</desc>
  <rect width="${WIDTH}" height="${HEIGHT}" fill="${theme.background}"/>
  <text x="${PADDING.left}" y="34" fill="${theme.foreground}" font-family="Arial, Helvetica, sans-serif" font-size="22" font-weight="700">${escapeXml(title)}</text>
  <text x="${WIDTH - PADDING.right}" y="34" fill="${theme.foreground}" font-family="Arial, Helvetica, sans-serif" font-size="18" font-weight="700" text-anchor="end">${starCount} stars</text>
  <text x="${WIDTH - PADDING.right}" y="55" fill="${theme.muted}" font-family="Arial, Helvetica, sans-serif" font-size="12" text-anchor="end">Latest star ${escapeXml(latestLabel)}</text>
  ${yTicks.map((tick) => gridLineY(tick, yFor(tick), plot, theme)).join('\n  ')}
  ${xTicks.map((tick) => gridLineX(tick, xFor(tick), plot, theme, formatXAxisLabel)).join('\n  ')}
  <path d="M ${plot.x},${plot.y} V ${plot.y + plot.height} H ${plot.x + plot.width}" fill="none" stroke="${theme.axis}" stroke-width="1"/>
  ${areaPath ? `<path d="${areaPath}" fill="${theme.area}" opacity="${theme.areaOpacity}"/>` : ''}
  <path d="${linePath}" fill="none" stroke="${theme.line}" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>
  ${points.length > 0 ? `<circle cx="${round(xFor(lastTime))}" cy="${round(yFor(starCount))}" r="4.5" fill="${theme.line}"/>` : ''}
  <text x="${plot.x + plot.width / 2}" y="${HEIGHT - 22}" fill="${theme.muted}" font-family="Arial, Helvetica, sans-serif" font-size="13" text-anchor="middle">Date</text>
  <text x="22" y="${plot.y + plot.height / 2}" fill="${theme.muted}" font-family="Arial, Helvetica, sans-serif" font-size="13" text-anchor="middle" transform="rotate(-90 22 ${plot.y + plot.height / 2})">Stars</text>
</svg>
`;
}

function gridLineY(value, y, plot, theme) {
  return `<line x1="${plot.x}" y1="${round(y)}" x2="${plot.x + plot.width}" y2="${round(y)}" stroke="${theme.grid}" stroke-width="1"/>
  <text x="${plot.x - 12}" y="${round(y + 4)}" fill="${theme.muted}" font-family="Arial, Helvetica, sans-serif" font-size="12" text-anchor="end">${value}</text>`;
}

function gridLineX(value, x, plot, theme, formatLabel) {
  return `<line x1="${round(x)}" y1="${plot.y}" x2="${round(x)}" y2="${plot.y + plot.height}" stroke="${theme.grid}" stroke-width="1"/>
  <text x="${round(x)}" y="${plot.y + plot.height + 24}" fill="${theme.muted}" font-family="Arial, Helvetica, sans-serif" font-size="12" text-anchor="middle">${escapeXml(formatLabel(new Date(value)))}</text>`;
}

function createDateTicks(minTime, maxTime, count) {
  if (count <= 1) {
    return [minTime];
  }
  return Array.from({ length: count }, (_, index) => Math.round(minTime + ((maxTime - minTime) * index) / (count - 1)));
}

function createNumberTicks(maxValue, count) {
  if (maxValue <= 1) {
    return [0, 1];
  }
  const step = niceStep(maxValue / (count - 1));
  const ticks = [];
  for (let value = 0; value <= maxValue; value += step) {
    ticks.push(value);
  }
  if (ticks.at(-1) !== maxValue) {
    ticks.push(maxValue);
  }
  return [...new Set(ticks)].sort((a, b) => a - b);
}

function niceStep(rawStep) {
  const exponent = Math.floor(Math.log10(rawStep));
  const base = 10 ** exponent;
  const fraction = rawStep / base;
  if (fraction <= 1) return base;
  if (fraction <= 2) return 2 * base;
  if (fraction <= 5) return 5 * base;
  return 10 * base;
}

function createXAxisFormatter(spanMs) {
  if (spanMs <= 120 * DAY_MS) {
    return createDateFormatter({ month: 'short', day: 'numeric', timeZone: 'UTC' });
  }
  if (spanMs <= 3 * 365 * DAY_MS) {
    return createDateFormatter({ month: 'short', year: 'numeric', timeZone: 'UTC' });
  }
  return createDateFormatter({ year: 'numeric', timeZone: 'UTC' });
}

function createDateFormatter(options) {
  const formatter = new Intl.DateTimeFormat('en', options);
  return (date) => formatter.format(date);
}

function formatDate(date) {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'UTC' }).format(date);
}

function escapeXml(value) {
  return String(value).replace(/[&<>"]/g, (character) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
  }[character]));
}

function round(value) {
  return Number(value.toFixed(2));
}

async function atomicWrite(filePath, contents) {
  await mkdir(dirname(filePath), { recursive: true });
  const tempPath = `${filePath}.tmp-${process.pid}`;
  try {
    await writeFile(tempPath, contents, 'utf8');
    const existing = await readFile(filePath, 'utf8').catch(() => null);
    if (existing === contents) {
      await unlink(tempPath);
      return;
    }
    await rename(tempPath, filePath);
  } catch (error) {
    await unlink(tempPath).catch(() => {});
    throw error;
  }
}
