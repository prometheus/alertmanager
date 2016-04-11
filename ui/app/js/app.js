'use strict';

angular.module('am.directives', []);

angular.module('am.directives').directive('route',
  function(RecursionHelper) {
    return {
      restrict: 'E',
      scope: {
        route: '='
      },
      templateUrl: 'app/partials/route.html',
      compile: function(element) {
        // Use the compile function from the RecursionHelper,
        // And return the linking function(s) which it returns
        return RecursionHelper.compile(element);
      }
    };
  }
);

angular.module('am.directives').directive('alert',
  function() {
    return {
      restrict: 'E',
      scope: {
        alert: '=',
        group: '='
      },
      templateUrl: 'app/partials/alert.html'
    };
  }
);

angular.module('am.directives').directive('silence',
  function() {
    return {
      restrict: 'E',
      scope: {
        sil: '='
      },
      templateUrl: 'app/partials/silence.html'
    };
  }
);

angular.module('am.directives').directive('silenceForm',
  function() {
    return {
      restrict: 'E',
      scope: {
        silence: '='
      },
      templateUrl: 'app/partials/silence-form.html'
    };
  }
);

angular.module('am.services', ['ngResource']);

angular.module('am.services').factory('Silence',
  function($resource) {
    return $resource('', {
      id: '@id'
    }, {
      'query': {
        method: 'GET',
        url: 'api/v1/silences'
      },
      'create': {
        method: 'POST',
        url: 'api/v1/silences'
      },
      'get': {
        method: 'GET',
        url: 'api/v1/silence/:id'
      },
      'delete': {
        method: 'DELETE',
        url: 'api/v1/silence/:id'
      }
    });
  }
);

angular.module('am.services').factory('Alert',
  function($resource) {
    return $resource('', {}, {
      'query': {
        method: 'GET',
        url: 'api/v1/alerts'
      }
    });
  }
);

angular.module('am.services').factory('AlertGroups',
  function($resource) {
    return $resource('', {}, {
      'query': {
        method: 'GET',
        url: 'api/v1/alerts/groups'
      }
    });
  }
);

angular.module('am.services').factory('Alert',
  function($resource) {
    return $resource('', {}, {
      'query': {
        method: 'GET',
        url: 'api/v1/alerts'
      }
    });
  }
);
angular.module('am.controllers', []);

angular.module('am.controllers').controller('NavCtrl',
  function($scope, $location) {
    $scope.items = [{
      name: 'Silences',
      url: 'silences'
    }, {
      name: 'Alerts',
      url: 'alerts'
    }, {
      name: 'Status',
      url: 'status'
    }];

    $scope.selected = function(item) {
      return item.url == $location.path()
    }
  }
);

angular.module('am.controllers').controller('AlertCtrl',
  function($scope) {
    $scope.showDetails = false;

    $scope.toggleDetails = function() {
      $scope.showDetails = !$scope.showDetails
    }

    $scope.showSilenceForm = false;

    $scope.toggleSilenceForm = function() {
      $scope.showSilenceForm = !$scope.showSilenceForm
    }

    $scope.silence = {
      matchers: []
    }
    angular.forEach($scope.alert.labels, function(value, key) {
      this.push({
        name: key,
        value: value,
        isRegex: false
      });
    }, $scope.silence.matchers);

    $scope.$on('silence-created', function(evt) {
      $scope.toggleSilenceForm();
    });
  }
);

angular.module('am.controllers').controller('AlertsCtrl',
  function($scope, $location, AlertGroups) {
    $scope.groups = null;
    $scope.allReceivers = [];

    $scope.$watch('receivers', function(recvs) {
      if (recvs === undefined || angular.equals(recvs, $scope.allReceivers)) {
        return;
      }
      if (recvs) {
        $location.search('receiver', recvs);
      } else {
        $location.search('receiver', null);
      }
    });

    $scope.notEmpty = function(group) {
      var l = 0;
      angular.forEach(group.blocks, function(blk) {
        if (this.indexOf(blk.routeOpts.receiver) >= 0) {
          l += blk.alerts.length || 0;
        }
      }, $scope.receivers);

      return l > 0;
    };

    $scope.refresh = function() {
      AlertGroups.query({},
        function(data) {
          $scope.groups = data.data;

          $scope.allReceivers = [];
          angular.forEach($scope.groups, function(group) {
            angular.forEach(group.blocks, function(blk) {
              if (this.indexOf(blk.routeOpts.receiver) < 0) {
                this.push(blk.routeOpts.receiver);
              }
            }, this);
          }, $scope.allReceivers);

          if (!$scope.receivers) {
            var recvs = angular.copy($scope.allReceivers);
            if ($location.search()['receiver']) {
              recvs = angular.copy($location.search()['receiver']);
              // The selected items must always be an array for multi-option selects.
              if (!angular.isArray(recvs)) {
                recvs = [recvs];
              }
            }
            $scope.receivers = recvs;
          }
        },
        function(data) {
          $scope.error = data.data;
        }
      );
    };

    $scope.refresh();
  }
);

angular.module('am.controllers').controller('SilenceCtrl',
  function($scope, $location, Silence) {

    $scope.highlight = $location.search()['hl'] == $scope.sil.id;

    $scope.showDetails = false;
    $scope.showSilenceForm = false;

    $scope.toggleSilenceForm = function() {
      $scope.showSilenceForm = !$scope.showSilenceForm
    }
    $scope.toggleDetails = function() {
      $scope.showDetails = !$scope.showDetails
    }

    var silCopy = angular.copy($scope.sil);

    $scope.delete = function(id) {
      Silence.delete({id: id},
        function(data) {
          $scope.$emit('silence-deleted');
        },
        function(data) {
          $scope.error = data.data;
        });
    };

    $scope.$on('silence-created', function(evt) {
      $scope.delete(silCopy.id);
    });
  }
);

angular.module('am.controllers').controller('SilencesCtrl',
  function($scope, Silence) {
    $scope.silences = [];
    $scope.order = "endsAt";

    $scope.showForm = false;

    $scope.toggleForm = function() {
      $scope.showForm = !$scope.showForm
    }

    $scope.refresh = function() {
      Silence.query({},
        function(data) {
          $scope.silences = data.data || [];

          angular.forEach($scope.silences, function(value) {
            value.endsAt = new Date(value.endsAt);
            value.startsAt = new Date(value.startsAt);
            value.createdAt = new Date(value.createdAt);
          });
        },
        function(data) {
          $scope.error = data.data;
        }
      );
    };

    $scope.$on('silence-created', function(evt) {
      $scope.refresh();
    });
    $scope.$on('silence-deleted', function(evt) {
      $scope.refresh();
    });

    $scope.elapsed = function(elapsed) {
      return function(sil) {
        if (elapsed) {
          return sil.endsAt <= new Date;
        }
        return sil.endsAt > new Date;
      }
    };

    $scope.refresh();
  }
);

angular.module('am.controllers').controller('SilenceCreateCtrl',
  function($scope, Silence) {
    $scope.error = null;
    $scope.silence = $scope.silence || {};

    if (!$scope.silence.matchers) {
      $scope.silence.matchers = [{}];
    }

    var origSilence = angular.copy($scope.silence);

    $scope.reset = function() {
      var now = new Date();
      var end = new Date();

      now.setMilliseconds(0);
      end.setMilliseconds(0);
      now.setSeconds(0);
      end.setSeconds(0);

      end.setHours(end.getHours() + 4)

      $scope.silence = angular.copy(origSilence);

      if (!origSilence.startsAt) {
        $scope.silence.startsAt = now;
      }
      if (!origSilence.endsAt) {
        $scope.silence.endsAt = end;
      }
    };

    $scope.reset();

    $scope.addMatcher = function() {
      $scope.silence.matchers.push({});
    };

    $scope.delMatcher = function(i) {
      $scope.silence.matchers.splice(i, 1);
    };

    $scope.create = function() {
      Silence.create($scope.silence,
        function(data) {
          $scope.$emit('silence-created');
          $scope.reset();
        },
        function(data) {
          $scope.error = data.data.error;
        }
      );
    };
  }
);

angular.module('am.services').factory('Status',
  function($resource) {
    return $resource('', {}, {
      'get': {
        method: 'GET',
        url: 'api/v1/status'
      }
    });
  }
);

angular.module('am.controllers').controller('StatusCtrl',
  function($scope, Status) {
    Status.get({},
      function(data) {
        $scope.config = data.data.config;
        $scope.versionInfo = data.data.versionInfo;
        $scope.uptime = data.data.uptime;
      },
      function(data) {
        console.log(data.data); 
      })
  }
);

angular.module('am', [
  'ngRoute',
  'ngSanitize',
  'angularMoment',

  'am.controllers',
  'am.services',
  'am.directives'
]);

angular.module('am').config(
  function($routeProvider) {
    $routeProvider.
    when('/alerts', {
      templateUrl: 'app/partials/alerts.html',
      controller: 'AlertsCtrl',
      reloadOnSearch: false
    }).
    when('/silences', {
      templateUrl: 'app/partials/silences.html',
      controller: 'SilencesCtrl',
      reloadOnSearch: false
    }).
    when('/status', {
      templateUrl: 'app/partials/status.html',
      controller: 'StatusCtrl'
    }).
    otherwise({
      redirectTo: '/alerts'
    });
  }
);
