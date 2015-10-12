'use strict';

angular.module('am.services', ['ngResource']);

angular.module('am.services').factory('Silence',
  function($resource){
    return $resource('', {}, {
      'get':    {method: 'GET', url: '/api/v1/silence/:id'},
      'save':   {method: 'POST', url: '/api/v1/silence/:id'},
      'query':  {method: 'GET', url: '/api/v1/silences'},
      'delete': {method: 'DELETE', url: '/api/v1/silence/:id'}
    });
  }
);

angular.module('am.controllers', []);

angular.module('am.controllers').controller('SilencesCtrl',
  function($scope, Silence) {
    $scope.silences = [];

    Silence.query(
      {},
      function(data) {
        $scope.silences = data.data || [];
      }
    );
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