'use strict';

angular.module('am.services', ['ngResource']);

angular.module('am.services').factory('Silence',
  function($resource){
    return $resource('', {id: '@id'}, {
      'query':  {method: 'GET', url: '/api/v1/silences'},
      'create': {method: 'POST', url: '/api/v1/silences'},
      'get':    {method: 'GET', url: '/api/v1/silence/:id'},
      'save':   {method: 'POST', url: '/api/v1/silence/:id'},
      'delete': {method: 'DELETE', url: '/api/v1/silence/:id'}
    });
  }
);

angular.module('am.services').factory('Alert',
  function($resource){
    return $resource('', {}, {
      'query':  {method: 'GET', url: '/api/v1/alerts'}
    });
  }
);

angular.module('am.controllers', []);

angular.module('am.controllers').controller('NavCtrl',
  function($scope, $location) {
    $scope.items = [
      {name: 'Silences', url:'/silences'},
      {name: 'Alerts', url:'/alerts'},
      {name: 'Status', url:'/status'}
    ];

    $scope.selected = function(item) {
      return item.url == $location.path()
    }    
  }
);

angular.module('am.controllers').controller('AlertsCtrl',
  function($scope, Alert) {
    $scope.alerts = [];
    $scope.order = "startsAt";

    $scope.refresh = function() {
      Alert.query({},
        function(data) {
          $scope.alerts = data.data || [];
        },
        function(data) {

        }
      );
    }

    $scope.refresh();
  }
);

angular.module('am.controllers').controller('SilencesCtrl',
  function($scope, Silence) {
    $scope.silences = [];
    $scope.order = "startsAt";

    $scope.refresh = function() {
      Silence.query({},
        function(data) {
          $scope.silences = data.data || [];
        },
        function(data) {

        }
      );
    }

    $scope.delete = function(sil) {
      Silence.delete({id: sil.id})
      $scope.refresh()
    }

    $scope.refresh();
  }
);

angular.module('am.controllers').controller('SilenceCreateCtrl',
  function($scope, Silence) {

    $scope.create = function() {
      Silence.create($scope.silence,
        function(data) {
          $scope.refresh();
        },
        function(data) {
          $scope.error = data.data;
        }
      );
    }

    $scope.error = null;

    $scope.newMatcher = function() {
      $scope.silence.matchers.push({});
    }
    $scope.delMatcher = function(i) {
      $scope.silence.matchers.splice(i, 1);
    }
    $scope.reset = function() {
      var now = new Date(),
          end = new Date();

      now.setMilliseconds(0);
      end.setMilliseconds(0);
      now.setSeconds(0);
      end.setSeconds(0);

      end.setHours(end.getHours() + 2)

      $scope.silence = {
        startsAt: now,
        endsAt: end,
        matchers: [{}],
        comment: "",
        createdBy: ""
      }
    }

    $scope.reset();
  }
);

angular.module('am', [
  'ngRoute',

  'am.controllers',
  'am.services'
]);

angular.module('am').config(
  function($routeProvider) {
    $routeProvider.
      when('/alerts', {
        templateUrl: '/app/partials/alerts.html',
        controller: 'AlertsCtrl'
      }).
      when('/silences', {
        templateUrl: '/app/partials/silences.html',
        controller: 'SilencesCtrl'
      }).
      otherwise({
        redirectTo: '/silences'
      });
  }
);
