module Views exposing (..)


-- External Imports
import Html exposing (..)
import Html.App as Html
import Html.Attributes exposing (..)
import Html.Events exposing (..)


-- Internal Imports
import Types exposing (Model, Silence, Alert, Msg, Route(..))


view : Model -> Html Msg
view model =
  case model.route of
    AlertsRoute ->
      genericListView todoView model.alerts

    AlertRoute name ->
      let
        one = Debug.log "view: name" name
      in
        todoView model.alert

    SilencesRoute ->
      genericListView silenceListView model.silences

    SilenceRoute name ->
      let
        one = Debug.log "view: name" name
      in
        silenceView model.silence

    _ ->
      notFoundView model


todoView : a -> Html Msg
todoView model =
  div [] [
    h1 [] [ text "todo" ]
  ]

notFoundView : a -> Html Msg
notFoundView model =
  div [] [
    h1 [] [ text "not found" ]
  ]


genericListView : (a -> Html Msg) -> List a -> Html Msg
genericListView fn list =
  div [ classList [ ("cf", True)
                  , ("pa2", True)
                  ]
  ] (List.map fn list)

silenceListView : Silence -> Html Msg
silenceListView silence =
  a [ class "db link dim tc"
    , href ("#/silence/" ++ silence.id) ]
    [ silenceView silence ]


silencesView : List Silence -> Html Msg
silencesView silences =
  div [ classList [ ("cf", True)
                  , ("pa2", True)
                  ]
  ] (List.map silenceListView silences)


silenceView : Silence -> Html msg
silenceView silence =
  div [ classList [ ("fl", True)
                  , ("w-50", False)
                  , ("pa2", True)
                  , ("w-25-m", True)
                  , ("w-w-20-l", True)
                  ]
    ]
    [ dl [ classList [ ("mt2", True)
                     , ("f6", True)
                     , ("lh-copy", True)
                     ]
      ] [ objectData silence.id
        , objectData silence.createdBy
        , objectData silence.comment
        ]
    ]


objectData : String -> Html msg
objectData data =
  dt [ class "m10 black truncate w-100" ] [ text data ]

