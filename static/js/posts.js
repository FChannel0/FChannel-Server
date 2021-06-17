function newpost()
{
    var el = document.getElementById("newpostbtn")
    var state = el.getAttribute("state")
    if(state = "0")
    {
        el.style="display:none;"
        el.setAttribute("state", "1")
        document.getElementById("newpost").style = "display: block;";        
    }
    else
    {
        el.style="display:block;"
        el.setAttribute("state", "0")
        document.getElementById("newpost").style = "display: hidden;";        
    }
}

function getMIMEType(type)
{
    re = /\/.+/g
    return type.replace(re, "")
}

function shortURL(actorName, url)
{
    re = /.+\//g;
    temp = re.exec(url)

    var output
    
    if(stripTransferProtocol(temp[0]) == stripTransferProtocol(actorName) + "/")
    {
        var short = url.replace("https://", "");
        short = short.replace("http://", "");
        short = short.replace("www.", "");

        var re = /^.{3}/g;

        var u = re.exec(short);

        re = /\w+$/g;              

        output =  re.exec(short);
    }else{
        var short = url.replace("https://", "");
        short = short.replace("http://", "");
        short = short.replace("www.", "");

        var re = /^.{3}/g;

        var u = re.exec(short);

        re = /\w+$/g;              

        u =  re.exec(short);

        str = short.replace(/\/+/g, " ")

        str = str.replace(u, " ").trim()

        re = /(\w|[!@#$%^&*<>])+$/;
        
        v = re.exec(str)

        output = "f" + v[0] + "-" + u
    }

    return output
}

function shortImg(url)
{
    var u = url;
    if(url.length > 26)
    {
        var re = /^.{26}/g;

        u = re.exec(url);

        re = /\..+$/g;

        var v = re.exec(url);

        u += "(...)" + v;
    }
    return u;        
}            

function convertSize(size)
{
    var convert = size / 1024.0;
    if(convert > 1024)
    {
        convert = convert / 1024.0
        convert = convert.toFixed(2) + " MB"
    }
    else
    {
        convert = convert.toFixed(2) + " KB"
    }

    return convert
}

function getBoardId(url)
{
    var re = /\/([^/\n]+)(.+)?/gm
    var matches = re.exec(url);
    return matches[1]
}

function convertContent(actorName, content, opid)
{
    var re = /(>>)(https?:\/\/)?(www\.)?.+\/\w+/gm;
    var match = content.match(re);
    var newContent = content;
    if(match)
    {
        match.forEach(function(quote, i){
            var link = quote.replace('>>', '')
            var isOP = ""
            if(link == opid)
            {
                isOP = " (OP)";
            }
            
            var q = link
            
            if(document.getElementById(link + "-content") != null) {
                q = document.getElementById(link + "-content").innerText;
                q = q.replaceAll('>', '/\>')
                q = q.replaceAll('"', '')
                q = q.replaceAll("'", "")                                
            }
            newContent = newContent.replace(quote, '<a class="reply" title="' + q +  '" href="'+ (actorName) + "/" + shortURL(actorName, opid)  +  '#' + shortURL(actorName, link) + '";">>>' + shortURL(actorName, link)  + isOP + '</a>');

        })            
    }
    
    re =  /^(\s+)?>.+/gm;

    match = newContent.match(re);
    if(match)
    {
        match.forEach(function(quote, i) {
    
            newContent = newContent.replace(quote, '<span class="quote">' + quote + '</span>');
        })
    }
    
    return newContent.replaceAll('/\>', '>')
}

function convertContentNoLink(actorName, content, opid)
{
    var re = /(>>)(https?:\/\/)?(www\.)?.+\/\w+/gm;
    var match = content.match(re);
    var newContent = content;
    if(match)
    {
        match.forEach(function(quote, i){
            var link = quote.replace('>>', '')
            var isOP = ""
            if(link == opid)
            {
                isOP = " (OP)";
            }
            
            var q = link
            
            if(document.getElementById(link + "-content") != null) {
                q = document.getElementById(link + "-content").innerText;
            }
            
            newContent = newContent.replace(quote, '>>' + shortURL(actorName, link)  + isOP);
        })            
    }
    newContent = newContent.replaceAll("'", "")
    return newContent.replaceAll('"', '')
}

function closeReply()
{
    document.getElementById("reply-box").style.display = "none";
    document.getElementById("reply-comment").value = "";        
}

function closeReport()
{
    document.getElementById("report-box").style.display = "none";
    document.getElementById("report-comment").value = "";        
}


function previous(actorName, page)
{
    var prev = parseInt(page) - 1;
    if(prev < 0)
        prev = 0;
    window.location.href = "/" + actorName + "/" + prev;
}

function next(actorName, totalPage, page)
{
    var next = parseInt(page) + 1;
    if(next > parseInt(totalPage))
        next = parseInt(totalPage);
    window.location.href = "/" + actorName + "/" + next;
}

function quote(actorName, opid, id)
{
    var box = document.getElementById("reply-box");
    var header = document.getElementById("reply-header");
    var comment = document.getElementById("reply-comment");
    var inReplyTo = document.getElementById("inReplyTo-box");      

    var w = window.innerWidth / 2 - 200;
    if(id == "reply") {
        var h = document.getElementById(id + "-content").offsetTop - 548;
    } else {
        var h = document.getElementById(id + "-content").offsetTop - 348;
    }


    box.setAttribute("style", "display: block; position: absolute; width: 400px; height: 550px; z-index: 9; top: " + h + "px; left: " + w + "px; padding: 5px;");


    if (inReplyTo.value != opid)
        comment.value = "";
    
    header.innerText = "Replying to Thread No. " + shortURL(actorName, opid);
    inReplyTo.value = opid;

    if(id != "reply")
        comment.value += ">>" + id + "\n";

    dragElement(header);            

}

function report(actorName, id)
{
    var box = document.getElementById("report-box");
    var header = document.getElementById("report-header");
    var comment = document.getElementById("report-comment");
    var inReplyTo = document.getElementById("report-inReplyTo-box");      

    var w = window.innerWidth / 2 - 200;
    var h = document.getElementById(id + "-content").offsetTop - 348;

    box.setAttribute("style", "display: block; position: absolute; width: 400px; height: 480px; z-index: 9; top: " + h + "px; left: " + w + "px; padding: 5px;");

    header.innerText = "Report Post No. " + shortURL(actorName, id);
    inReplyTo.value = id;

    dragElement(header);            
}  

function dragElement(elmnt) {
    var pos1 = 0, pos2 = 0, pos3 = 0, pos4 = 0;
    
    elmnt.onmousedown = dragMouseDown;

    function dragMouseDown(e) {
        e = e || window.event;
        e.preventDefault();
        // get the mouse cursor position at startup:
        pos3 = e.clientX;
        pos4 = e.clientY;
        document.onmouseup = closeDragElement;
        // call a function whenever the cursor moves:
        document.onmousemove = elementDrag;
    }

    function elementDrag(e) {
        e = e || window.event;
        e.preventDefault();
        // calculate the new cursor position:
        pos1 = pos3 - e.clientX;
        pos2 = pos4 - e.clientY;
        pos3 = e.clientX;
        pos4 = e.clientY;
        // set the element's new position:
        elmnt.parentElement.style.top = (elmnt.parentElement.offsetTop - pos2) + "px";
        elmnt.parentElement.style.left = (elmnt.parentElement.offsetLeft - pos1) + "px";
    }

    function closeDragElement() {
        // stop moving when mouse button is released:
        document.onmouseup = null;
        document.onmousemove = null;
    }
}

function stripTransferProtocol(value){
    var re = /(https:\/\/|http:\/\/)?(www.)?/

    return value.replace(re, "")
}

