function setCookie(key, value, age) {
	document.cookie = key + "=" + encodeURIComponent(value) + ";sameSite=strict;max-age=" + 60 * 60 * 24 * age + ";path=/";
}

function getCookie(key) {
	if (document.cookie.length != 0) {
		return document.cookie.split('; ').find(row => row.startsWith(key)).split('=')[1];
	}
	return "";
}

function setTheme(name) {
	for (let i = 0, tags = document.getElementsByTagName("link"); i < tags.length; i++) {
		if (tags[i].type = "text/css" && tags[i].title) {
			tags[i].disabled = !(tags[i].title == name);
		}
	}

	setCookie("theme", name, 3650);
}

function applyTheme() {
	setTheme(getCookie("theme") || "default");
}
