var timerCount;
var timerToggle = false;
var timer;
const contentLoadHandler = function(event){
    timerToggle = !!document.getElementById("autoreload-checkbox").checked;
    if(timerToggle){
        timerCount = 45;
        document.getElementById("autoreload-countdown").innerHTML = "45";
        document.getElementById("autoreload-countdown").style.visibility = "visible";
        timer = setInterval(timerFunction, 1000);
        document.removeEventListener("DOMContentLoaded", contentLoadHandler, false);
    }
};

document.addEventListener("DOMContentLoaded", contentLoadHandler, false);

function timerFunction(){
    timerCount--;
    document.getElementById("autoreload-countdown").innerHTML = timerCount;
    if(timerCount <= 0){
        document.getElementById("autoreload-countdown").innerHTML = "Refreshing...";
        clearInterval(timer);
        location.reload();
    }
}

function autoTimer(){
    timerToggle = !timerToggle;
    if(timerToggle === true){
        timerCount = 45;
        document.getElementById("autoreload-countdown").innerHTML = "45";
        document.getElementById("autoreload-countdown").style.visibility = "visible";
        timer = setInterval(timerFunction, 1000);
    }else{
        clearInterval(timer);
        document.getElementById("autoreload-countdown").style.visibility = "hidden";
    }
}
