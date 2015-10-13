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

angular.module('am.controllers', []);

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
      matchers: [{}]
    }

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
      when('/silences', {
        templateUrl: '/app/partials/silences.html',
        controller: 'SilencesCtrl'
      }).
      otherwise({
        redirectTo: '/silences'
      });
  }
);