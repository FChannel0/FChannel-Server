var imgs = document.querySelectorAll('#img');
var imgArray = [].slice.call(imgs);

imgArray.forEach(function(img, i){
    img.addEventListener("click", function(e){
        if(img.getAttribute("enlarge") == "0")
        {
            var attachment = img.getAttribute("attachment")
            img.setAttribute("enlarge", "1");
            img.setAttribute("style", "float: left; margin-right: 10px; cursor: move;");
            img.src = attachment
        }
        else
        {
            var preview = img.getAttribute("preview")
            img.setAttribute("enlarge", "0");
            if(img.getAttribute("main") == 1)
            {
                img.setAttribute("style", "float: left; margin-right: 10px; max-width: 250px; max-height: 250px; cursor: move;");
                img.src = preview
            }
            else
            {
                img.setAttribute("style", "float: left; margin-right: 10px; max-width: 125px; max-height: 125px; cursor: move;");
                img.src = preview                
            }
        }
    });
})


function viewLink(board, actor) {
    var posts = document.querySelectorAll('#view');
    var postsArray = [].slice.call(posts);

    postsArray.forEach(function(p, i){
        var id = p.getAttribute("post")
        p.href = "/" + board + "/" + shortURL(actor, id)
    })  
}
