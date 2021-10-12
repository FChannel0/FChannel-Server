function setCookie(key, value, age) {
	document.cookie = key + "=" + encodeURIComponent(value) + ";sameSite=strict;max-age=" + (60 * 60 * 24 * age) + ";path=/";
}

function getCookie(key) {
	if (document.cookie.length != 0) {
		return document.cookie.split('; ').find(row => row.startsWith(key)).split('=')[1];
	}
	return "";
}

function setTheme(name) {
	for (let i = 0, tags = document.getElementsByTagName("link"); i < tags.length; i++) {
		if (tags[i].type === "text/css" && tags[i].title) {
			tags[i].disabled = !(tags[i].title === name);
		}
	}

	setCookie("theme", name, 3650);
}

function applyTheme() {
	// HACK: disable all of the themes first. this for some reason makes things work.
	for (let i = 0, tags = document.getElementsByTagName("link"); i < tags.length; i++) {
		if (tags[i].type === "text/css" && tags[i].title) {
			tags[i].disabled = true;
		}
	}
	let theme = getCookie("theme") || "default";
	setTheme(theme);

	// reflect this in the switcher
	let switcher = document.getElementById("themeSwitcher");
	for(var i = 0; i < switcher.options.length; i++) {
		if (switcher.options[i].value === theme) {
			switcher.selectedIndex = i;
			break;
		}
	}
}
