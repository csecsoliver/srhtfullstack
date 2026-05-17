function UserAutoComplete(input, list) {
  this.input = input;
  this.list = list;
  this.lastQuery = input.value;

  // Settings
  this.delay = 250;
  this.minQueryLength = 3;

  this.onLoad = function(event) {
    const response = event.target;
    if (response.status == 200) {
      const data = JSON.parse(response.responseText);

      this.list.innerHTML = '';
      data.results.forEach(function (username) {
        const option = document.createElement('option');
        option.value = username;
        this.list.appendChild(option);
      }.bind(this));
    }
  }.bind(this)

  this.sendRequest = function(query) {
    const search = encodeURIComponent(query);
    const request = new XMLHttpRequest();
    request.onload = this.onLoad;
    request.open("GET", "/usernames/?q=" + search);
    request.send();
  }

  this.search = function() {
    const query = this.input.value
    if (query == "" || query == "~") {
      this.list.innerHTML = "";
      this.lastQuery = "";
      return;
    }

    const notRepeated = query !== this.lastQuery;
    const notTooShort = query.length >= this.minQueryLength;
    if (notRepeated && notTooShort) {
      this.sendRequest(query);
      this.lastQuery = query;
    }
  }.bind(this)

  this.debounce = function(fn, delay) {
    let timeout = null;
    return function() {
      if (timeout) {
        clearTimeout(timeout);
      }
      timeout = setTimeout(fn, delay);
    }
  }

  this.register = function() {
    this.input.addEventListener("input",
        this.debounce(this.search, this.delay));

    // Prevent search being triggered when an user is selected from the datalist
    // 'select' works in Firefox, 'change' works in Chrome
    this.input.addEventListener("select", function(e) {
      this.lastQuery = e.target.value;
    }.bind(this));

    this.input.addEventListener("change", function(e) {
      this.lastQuery = e.target.value;
    }.bind(this));
  }
}
