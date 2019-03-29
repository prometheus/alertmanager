var a;

$(function() {
  $("#configs-form").submit(function(e) {
    e.preventDefault();
    $.post("http://localhost:12345/preview", $("#configs-form").serialize())
    .done(function(data) {
      $("#error-message").hide();
      window.open("https://api.slack.com/docs/messages/builder?msg=" + encodeURIComponent(data), "_blank");
      //window.location.href = "https://api.slack.com/docs/messages/builder?msg=" + encodeURIComponent(data);
    })
    .fail(function(resp) {
      var msg;
      if (resp.status === 0) {
        msg = "Error sending request"
      } else {
        var msg = resp.statusText;
        if (resp.responseText) {
          msg += ": " + resp.responseText;
        }
      }
      $("#error-message").text(msg);
      $("#error-message").show();
    })
    .always(function() {
      //alert( "finished" );
    });
  })
})
