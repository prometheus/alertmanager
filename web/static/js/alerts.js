function clearSilenceLabels() {
  $("#silence_label_table").empty();
}

function addSilenceLabel(label, value) {
  if (!label) {
    label = "";
  }
  if (!value) {
    value = "";
  }
  $("#silence_label_table").append(
        '<tr>' +
        '  <td><input class="input-large" type="text" placeholder="label regex" value="' + label + '"></td>' +
        '  <td><input class="input-large" type="text" placeholder="value regex" value="' + value + '"></td>' +
        '  <td><button class="btn del_label_button"><i class="icon-minus"></i></button></td>' +
        '</tr>');
  bindDelLabel();
}

function bindDelLabel() {
  $(".del_label_button").unbind("click");
  $(".del_label_button").click(function() {
    $(this).parents("tr").remove();
  });
}

function init() {
  $("#new_silence_btn").click(function() {
    clearSilenceLabels();
  });

  $(".add_silence_btn").click(function() {
    clearSilenceLabels();

    var form = $(this).parents("form");
    var labels = form.children('input[name="label[]"]');
    var values = form.children('input[name="value[]"]');
    for (var i = 0; i < labels.length; i++) {
      addSilenceLabel(labels.get(i).value, values.get(i).value);
    }
  });

  $("#add_label_button").click(function() {
    addSilenceLabel("", "");
  });
}

$(init);
